package swarm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/yiduoyunQ/smlib"
)

var (
	ErrServiceNotFound = errors.New("Service Not Found")
)

type Service struct {
	sync.RWMutex

	failureRetry int64

	database.Service
	base *structs.PostServiceRequest

	pendingContainers map[string]*pendingContainer

	units  []*unit
	users  []database.User
	backup *database.BackupStrategy
	task   *database.Task

	authConfig *types.AuthConfig
}

func NewService(svc database.Service, unitNum int) *Service {
	return &Service{
		Service:           svc,
		units:             make([]*unit, 0, unitNum),
		pendingContainers: make(map[string]*pendingContainer, unitNum),
	}
}

func BuildService(req structs.PostServiceRequest, authConfig *types.AuthConfig) (*Service, error) {
	if warnings := ValidService(req); len(warnings) > 0 {
		return nil, errors.New(strings.Join(warnings, ","))
	}

	des, err := json.Marshal(req)
	if err != nil {
		log.Errorf("JSON Marshal Error:%s", err.Error())
		return nil, err
	}

	svc := database.Service{
		ID:                   utils.Generate64UUID(),
		Name:                 req.Name,
		Description:          string(des),
		Architecture:         req.Architecture,
		AutoHealing:          req.AutoHealing,
		AutoScaling:          req.AutoScaling,
		HighAvailable:        req.HighAvailable,
		Status:               _StatusServiceInit,
		BackupMaxSizeByte:    req.BackupMaxSize,
		BackupFilesRetention: int(req.BackupRetention * time.Second),
		CreatedAt:            time.Now(),
	}

	strategy, err := newBackupStrategy(svc.ID, req.BackupStrategy)
	if err != nil {
		return nil, err
	}

	_, nodeNum, err := getServiceArch(req.Architecture)
	if err != nil {
		log.Error("Parse Service.Architecture", err)
		return nil, err
	}

	service := NewService(svc, nodeNum)

	service.Lock()
	defer service.Unlock()

	service.task = database.NewTask("service", svc.ID, "create service", nil, 0)

	service.backup = strategy
	service.base = &req
	service.authConfig = authConfig
	service.users = converteToUsers(service.ID, req.Users)
	atomic.StoreInt64(&svc.Status, _StatusServcieBuilding)

	if err := service.SaveToDB(); err != nil {
		atomic.StoreInt64(&svc.Status, _StatusServiceInit)

		log.Error("Service Save To DB", err)
		return nil, err
	}

	return service, nil
}

func newBackupStrategy(service string, strategy *structs.BackupStrategy) (*database.BackupStrategy, error) {
	if strategy == nil {
		return nil, nil
	}

	valid, err := utils.ParseStringToTime(strategy.Valid)
	if err != nil {
		log.Error("Parse Request.BackupStrategy.Valid to time.Time", err)
		return nil, err
	}

	return &database.BackupStrategy{
		ID:        utils.Generate64UUID(),
		Type:      strategy.Type,
		ServiceID: service,
		Spec:      strategy.Spec,
		Valid:     valid,
		Enabled:   true,
		BackupDir: strategy.BackupDir,
		Timeout:   int(strategy.Timeout * time.Second),
		CreatedAt: time.Now(),
	}, nil
}

func ValidService(req structs.PostServiceRequest) []string {
	warnings := make([]string, 0, 10)
	if req.Name == "" {
		warnings = append(warnings, "Service Name should not be null")
	}

	arch, _, err := getServiceArch(req.Architecture)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Parse 'Architecture' Failed,%s", err.Error()))
	}

	for _, module := range req.Modules {
		if !isStringExist(module.Type, supportedServiceTypes) {
			warnings = append(warnings, fmt.Sprintf("Unsupported '%s' Yet", module.Type))
		}
		if module.Config.Image == "" {
			image, err := database.QueryImage(module.Name, module.Version)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Not Found Image:%s:%s,Error%s", module.Name, module.Version, err.Error()))
			}
			if !image.Enabled {
				warnings = append(warnings, fmt.Sprintf("Image: %s:%s is Disabled", module.Name, module.Version))
			}
		} else {
			image, err := database.QueryImageByID(module.Config.Image)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Not Found Image:%s,Error%s", module.Config.Image, err.Error()))
			}
			if !image.Enabled {
				warnings = append(warnings, fmt.Sprintf("Image:%s is Disabled", module.Config.Image))
			}
		}
		_, num, err := getServiceArch(module.Arch)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s,%s", module.Arch, err))
		}

		if arch[module.Type] != num {
			warnings = append(warnings, fmt.Sprintf("%s nodeNum  unequal Architecture,(%s)", module.Type, module.Arch))
		}

		config := cluster.BuildContainerConfig(module.Config, module.HostConfig, module.NetworkingConfig)
		err = validateContainerConfig(config)
		if err != nil {
			warnings = append(warnings, err.Error())
		}

		lvNames := make([]string, 0, len(module.Stores))
		for _, ds := range module.Stores {
			if isStringExist(ds.Name, lvNames) {
				warnings = append(warnings, fmt.Sprintf("Storage Name '%s' Duplicate in one Module:%s", ds.Name, module.Name))
			} else {
				lvNames = append(lvNames, ds.Name)
			}

			if !isStringExist(ds.Name, supportedStoreNames) {
				warnings = append(warnings, fmt.Sprintf("Unsupported Storage Name '%s' Yet,should be one of %s", ds.Name, supportedStoreNames))
			}

			if !isStringExist(ds.Type, supportedStoreTypes) {
				warnings = append(warnings, fmt.Sprintf("Unsupported Storage Type '%s' Yet,should be one of %s", ds.Type, supportedStoreTypes))
			}
		}
	}

	if len(warnings) == 0 {
		return nil
	}

	log.Warnf("Service Valid warning:", warnings)

	return warnings
}

func (svc *Service) BackupStrategy() *database.BackupStrategy {
	svc.RLock()
	backup := svc.backup
	svc.RUnlock()

	return backup
}

func (svc *Service) ReplaceBackupStrategy(req structs.BackupStrategy) (*database.BackupStrategy, error) {
	backup, err := newBackupStrategy(svc.ID, &req)
	if err != nil || backup == nil {
		return nil, fmt.Errorf("With non Backup Strategy,Error:%v", err)
	}

	err = database.InsertBackupStrategy(backup)
	if err != nil {
		return nil, fmt.Errorf("Tx Insert Backup Strategy Error:%s", err.Error())
	}

	svc.Lock()
	svc.backup = backup
	svc.Unlock()

	return backup, nil
}

func (gd *Gardener) DeleteServiceBackupStrategy(strategy string) error {
	backup, err := database.GetBackupStrategy(strategy)
	if err != nil {
		return err
	}

	if backup.Enabled {
		return fmt.Errorf("Backup Strategy %s is using,Cannot Delete", strategy)
	}

	err = database.DeleteBackupStrategy(strategy)

	return err
}

func converteToUsers(service string, users []structs.User) []database.User {
	list := make([]database.User, len(users))
	for i := range users {
		list[i] = database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      users[i].Type,
			Username:  users[i].Username,
			Password:  users[i].Password,
			Role:      users[i].Role,
			CreatedAt: time.Now(),
		}
	}

	return list
}

func (svc *Service) getUnit(IDOrName string) (*unit, error) {
	for i := range svc.units {
		if svc.units[i].ID == IDOrName || svc.units[i].Name == IDOrName {
			return svc.units[i], nil
		}
	}

	return nil, fmt.Errorf("Unit Not Found,%s", IDOrName)
}

func (gd *Gardener) AddService(svc *Service) error {
	if svc == nil {
		return errors.New("Service Cannot be nil pointer")
	}

	gd.RLock()
	for i := range gd.services {
		if gd.services[i].ID == svc.ID || gd.services[i].Name == svc.Name {
			gd.RUnlock()

			return fmt.Errorf("Service %s Existed", svc.Name)
		}
	}
	gd.RUnlock()

	gd.Lock()
	gd.services = append(gd.services, svc)
	gd.Unlock()

	return nil
}

func (svc *Service) SaveToDB() error {
	return database.TxSaveService(&svc.Service, svc.backup, svc.task, svc.users)
}

func (gd *Gardener) GetService(NameOrID string) (*Service, error) {
	gd.RLock()

	for i := range gd.services {
		if gd.services[i].ID == NameOrID || gd.services[i].Name == NameOrID {
			gd.RUnlock()

			return gd.services[i], nil
		}
	}

	gd.RUnlock()

	return gd.rebuildService(NameOrID)
}

func (gd *Gardener) rebuildService(NameOrID string) (*Service, error) {
	service, err := database.GetService(NameOrID)
	if err != nil {
		return nil, err
	}

	base := &structs.PostServiceRequest{}

	if len(service.Description) > 0 {
		err := json.Unmarshal([]byte(service.Description), base)
		if err != nil {
			log.Warnf("JSON Unmarshal Service.Description Error:%s,Description:%s", err.Error(), service.Description)
		}
	}

	var backup *database.BackupStrategy
	strategies, err := database.GetBackupStrategyByServiceID(service.ID)
	if err != nil {
		return nil, err
	}

	for i := range strategies {
		if strategies[i].Enabled {
			backup = &strategies[i]
			break
		}
	}

	units, err := database.ListUnitByServiceID(service.ID)
	if err != nil {
		return nil, err
	}
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		log.Errorf("Registry Auth Config Error:%s", err.Error())
		return nil, err
	}

	// TODO:rebuild units

	svc := NewService(service, len(units))
	svc.Lock()
	svc.backup = backup
	svc.base = base
	svc.authConfig = authConfig
	svc.Unlock()

	gd.Lock()
	gd.services = append(gd.services, svc)
	gd.Unlock()

	return svc, nil

}

func (gd *Gardener) CreateService(req structs.PostServiceRequest) (_ *Service, err error) {
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		log.Error("get Registry Auth Config", err)
		return nil, err
	}

	svc, err := BuildService(req, authConfig)
	if err != nil {
		log.Error("Build Service", err)
		return nil, err
	}

	defer func() {
		if err != nil {
			if svc.backup != nil && svc.backup.ID != "" {
				gd.RemoveCronJob(svc.backup.ID)
			}
			client, err := gd.consulAPIClient(true)
			if err != nil {
			}
			svc.Delete(client, true, true, 0)
			log.WithField("Service Name", svc.Name).Errorf("Servcie Cleaned,%v", err)
		}
	}()

	log.WithFields(log.Fields{
		"Servcie Name": svc.Name,
		"Service ID":   svc.ID,
		"Task ID":      svc.task.ID,
	}).Info("Service Saved Into Database")

	svc.failureRetry = gd.createRetry

	if err := gd.AddService(svc); err != nil {
		log.WithField("Service Name", svc.Name).Errorf("Service Add to Gardener Error:%s", err.Error())

		return svc, err
	}

	if svc.backup != nil {
		bs := NewBackupJob(gd.host, svc)
		err := gd.RegisterBackupStrategy(bs)
		if err != nil {
			log.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err.Error())
		}
	}

	if err := gd.ServiceToScheduler(svc); err != nil {
		log.Error("Service Add To Scheduler", err)
		return svc, err
	}
	log.Debugf("[mg] ServiceToScheduler ok:%v", svc)

	return svc, nil
}

func (svc *Service) StartService() error {
	err := svc.statusCAS(_StatusServiceNoContent, _StatusServiceStarting)
	if err != nil {
		return err
	}
	svc.Lock()
	defer func() {
		if err != nil {
			svc.SetServiceStatus(_StatusServiceStartFailed, time.Now())
		} else {
			atomic.StoreInt64(&svc.Status, _StatusServiceNoContent)
		}
		svc.Unlock()
	}()

	err = svc.startContainers()
	if err != nil {
		return err
	}

	err = svc.startService()
	if err != nil {
		return err
	}

	err = svc.refreshTopology()
	if err != nil {
		return err
	}

	return nil
}
func (svc *Service) startContainers() error {
	for i := range svc.units {
		err := svc.units[i].startContainer()
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) copyServiceConfig() error {
	for i := range svc.units {
		u := svc.units[i]

		defConfig, err := u.defaultUserConfig(svc, u)
		if err != nil {
			return err
		}

		err = u.CopyConfig(defConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) initService() error {
	for i := range svc.units {
		err := svc.units[i].initService()
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) checkStatus(expected int64) error {
	val := atomic.LoadInt64(&svc.Status)
	if val == expected {
		return nil
	}

	return fmt.Errorf("Status Conflict:expected %d but got %d", expected, val)
}

func (svc *Service) statusCAS(expected, value int64) error {
	if atomic.CompareAndSwapInt64(&svc.Status, expected, value) {
		return nil
	}

	return fmt.Errorf("Status Conflict:expected %d but got %d", expected, atomic.LoadInt64(&svc.Status))
}

func (svc *Service) startService() error {
	for i := range svc.units {
		err := svc.units[i].startService()
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) StopContainers(timeout int) error {
	svc.Lock()
	err := svc.stopContainers(timeout)
	svc.Unlock()

	return err
}

func (svc *Service) stopContainers(timeout int) error {
	for i := range svc.units {
		err := svc.units[i].stopContainer(timeout)
		if err != nil {
			return err
		}
	}

	return nil
}
func (svc *Service) StopService() error {
	svc.Lock()
	err := svc.stopService()
	svc.Unlock()

	return err
}

func (svc *Service) stopService() error {
	for i := range svc.units {
		err := svc.units[i].stopService()
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) RemoveContainers(force, rmVolumes bool) error {
	svc.Lock()
	err := svc.stopService()
	if err != nil {
		return err
	}
	err = svc.removeContainers(force, rmVolumes)
	svc.Unlock()

	return err
}

func (svc *Service) removeContainers(force, rmVolumes bool) error {
	for i := range svc.units {
		err := svc.units[i].removeContainer(force, rmVolumes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) createUsers() error {
	users := []database.User{}
	cmd := []string{}

	// TODO:edit cmd
	for i := range users {
		cmd[i] = users[i].Username
	}

	for i := range svc.units {
		u := svc.units[i]

		if u.Type == _MysqlType {
			err := containerExec(u.engine, u.ContainerID, cmd, false)
			if err != nil {

			}
		}
	}

	svc.getUnitByType(_UnitRole_SwitchManager)

	return nil
}

func (svc *Service) refreshTopology() error {
	svc.getUnitByType(_UnitRole_SwitchManager)

	return nil
}

func (svc *Service) initTopology() error {
	svc.getUnitByType(_UnitRole_SwitchManager)

	return nil
}

func (svc *Service) registerServices(client *consulapi.Client) (err error) {
	if client == nil {
		return fmt.Errorf("consul client is nil")
	}

	for i := range svc.units {
		err = svc.units[i].RegisterHealthCheck(client)
		if err != nil {

			return err
		}
	}

	return nil
}

func (svc *Service) DeregisterServices(client *consulapi.Client) error {
	if client == nil {
		return fmt.Errorf("consul client is nil")
	}
	svc.Lock()

	for i := range svc.units {
		err := svc.units[i].DeregisterHealthCheck(client)
		if err != nil {
			svc.Unlock()
			return err
		}
	}
	svc.Unlock()

	return nil
}

func (svc *Service) registerToHorus(addr, user, password string, agentPort int) error {
	for i := range svc.units {
		err := svc.units[i].registerToHorus(addr, user, password, agentPort)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) getUnitByType(Type string) (*unit, error) {
	for i := range svc.units {
		if svc.units[i].Type == Type {
			return svc.units[i], nil
		}
	}

	return nil, fmt.Errorf("Not Found unit %s In Service %s", Type, svc.ID)
}

func (svc *Service) GetSwithManager() (*unit, error) {
	svc.RLock()
	u, err := svc.getUnitByType(_UnitRole_SwitchManager)
	svc.RUnlock()

	return u, err
}

func (svc *Service) getSwithManagerAddr() (addr string, port int, err error) {
	u, err := svc.getUnitByType(_UnitRole_SwitchManager)
	if err != nil {
		return "", 0, err
	}

	for i := range u.networkings {
		if u.networkings[i].Type == _ContainersNetworking {
			addr = u.networkings[i].IP.String()

			break
		}
	}

	for i := range u.ports {
		if strings.EqualFold(u.ports[i].Name, "Port") {
			port = u.ports[i].Port

			return addr, port, nil
		}
	}

	return addr, port, fmt.Errorf("Not Found")
}

func (svc *Service) GetSwitchManagerAndMaster() (string, int, *unit, error) {
	svc.RLock()
	defer svc.RUnlock()

	addr, port, err := svc.getSwithManagerAddr()
	if err != nil {
		return addr, port, nil, err
	}

	topology, err := smlib.GetTopology(addr, port)
	if err != nil {
		return addr, port, nil, err
	}

	masterID := ""
loop:
	for _, val := range topology.DataNodeGroup {
		for id, node := range val {
			if strings.EqualFold(node.Type, _UnitRole_Master) {
				masterID = id

				break loop
			}
		}
	}

	if masterID == "" {
		// Not Found master DB
		return addr, port, nil, fmt.Errorf("Master Unit Not Found")
	}

	master, err := svc.getUnit(masterID)

	return addr, port, master, err
}

func (svc *Service) TryBackupTask(host, unitID, strategyID, strategyType string, timeout int) error {
	task := database.NewTask("backup_strategy", strategyID, "", nil, timeout)
	task.Status = _StatusTaskCreate

	err := database.InsertTask(task)
	if err != nil {
		return err
	}

	backup := &unit{}

	for retries := 3; ; retries-- {
		if retries != 3 {
			time.Sleep(time.Second * 60)
		}

		addr, port, master, err := svc.GetSwitchManagerAndMaster()
		if err != nil {
			if retries > 0 {
				continue
			}
			err1 := database.UpdateTaskStatus(task, _StatusTaskCancel, time.Now(), "Cancel,The Task marked as TaskCancel,"+err.Error())
			return fmt.Errorf("Errors:%v,%v", err, err1)
		}

		if err := smlib.Lock(addr, port); err != nil {
			if retries > 0 {
				continue
			}
			err1 := database.UpdateTaskStatus(task, _StatusTaskCancel, time.Now(), "TaskCancel,Switch Manager is busy now,"+err.Error())
			return fmt.Errorf("Errors:%v,%v", err, err1)
		}

		backup = master
		defer smlib.UnLock(addr, port)

		break
	}

	if unitID != "" {
		svc.RLock()
		backup, err = svc.getUnit(unitID)
		svc.RUnlock()

		if err != nil {
			return err
		}
	}

	args := []string{host + "v1.0/task/backup/callback", task.ID, strategyID, backup.ID, strategyType}

	errCh := make(chan error, 1)

	select {
	case errCh <- backup.backup(args...):

	case <-time.After(time.Duration(timeout)):

		err = database.UpdateTaskStatus(task, _StatusTaskTimeout, time.Now(), "Timeout,The Task marked as TaskTimeout")

		return fmt.Errorf("Task Timeout,Service:%s,BackupStrategy:%s,Task:%s,Error:%v", svc.ID, strategyID, task.ID, err)
	}

	msg := <-errCh
	if msg == nil {
		return nil
	}

	return fmt.Errorf("Backup %s Task Faild,%v", unitID, msg)
}

func (svc *Service) Task() *database.Task {
	svc.RLock()
	task := svc.task
	svc.RUnlock()

	return task
}

func (gd *Gardener) DeleteService(name string, force, volumes bool, timeout int) error {
	svc, err := gd.GetService(name)
	if err != nil {
		if err == ErrServiceNotFound {
			return nil
		}

		return err
	}

	client, err := gd.consulAPIClient(true)
	if err != nil {

	}

	err = svc.Delete(client, force, volumes, timeout)
	if err != nil {
		return err
	}

	err = gd.RemoveCronJob(svc.backup.ID)

	return err

}

func (svc *Service) Delete(client *consulapi.Client, force, volumes bool, timeout int) error {
	svc.Lock()
	defer svc.Unlock()

	err := svc.stopContainers(timeout)
	if err != nil {
		return err
	}

	err = svc.removeContainers(force, volumes)
	if err != nil {
		return err
	}

	err = svc.DeregisterServices(client)

	return err
}
