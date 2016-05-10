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

	strategy, err := newBackupStrategy(req.BackupStrategy)
	if err != nil {
		return nil, err
	}

	strategyID := ""
	if strategy != nil {
		strategyID = strategy.ID
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
		BackupStrategyID:     strategyID,
		BackupMaxSizeByte:    req.BackupMaxSize,
		BackupFilesRetention: int(req.BackupRetention * time.Second),
		CreatedAt:            time.Now(),
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

func newBackupStrategy(strategy *structs.BackupStrategy) (*database.BackupStrategy, error) {
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
			_, err := database.QueryImage(module.Name, module.Version)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Not Found Image:%s:%s,Error%s", module.Name, module.Version, err.Error()))
			}
		} else {
			_, err := database.QueryImageByID(module.Config.Image)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Not Found Image:%s,Error%s", module.Config.Image, err.Error()))
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
			for i := range lvNames {
				if lvNames[i] == ds.Name {
					warnings = append(warnings, fmt.Sprintf("Storage Name '%s' Duplicate in one Module:%s", ds.Name, module.Name))
				}
			}

			lvNames = append(lvNames, ds.Name)
		}
	}

	if len(warnings) == 0 {
		return nil
	}

	log.Warnf("Service Valid warning:", warnings)

	return warnings
}

func (svc *Service) ReplaceBackupStrategy(req structs.BackupStrategy) (*database.BackupStrategy, error) {
	backup, err := newBackupStrategy(&req)
	if err != nil || backup == nil {
		return nil, fmt.Errorf("With Non BackupStrategy,Error:%v", err)
	}

	tx, err := database.GetTX()
	if err != nil {
		return nil, fmt.Errorf("DB TX Error:%s", err.Error())
	}
	defer tx.Rollback()

	err = database.TxInsertBackupStrategy(tx, backup)
	if err != nil {
		return nil, fmt.Errorf("Tx Insert Backup Strategy Error:%s", err.Error())
	}

	err = database.TxUpdateServiceBackupStrategy(tx, svc.ID, svc.BackupStrategyID, backup.ID)
	if err != nil {
		return nil, fmt.Errorf("Tx Update Service Backup Strategy Error:%s", err.Error())
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	svc.Lock()
	svc.backup = backup
	svc.BackupStrategyID = backup.ID
	svc.Unlock()

	return backup, nil
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
	if service.BackupStrategyID != "" {
		backup, err = database.GetBackupStrategy(service.BackupStrategyID)
		if err != nil {
			return nil, err
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
			svc.Delete(true, true, 0)
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
	log.Debugf("[**MG**] ServiceToScheduler ok:%v", svc)
	return svc, nil
}

func (svc *Service) CreateContainers() (err error) {
	if val := atomic.LoadInt64(&svc.Status); val != _StatusServiceCreating {
		return fmt.Errorf("Status Conflict,%d!=_StatusServiceCreating", val)
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, _StatusServiceCreateFailed)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].prepareCreateContainer()
		if err != nil {
			return err
		}

		_, err = svc.units[i].createContainer(svc.authConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) StartContainers() (err error) {
	if val := atomic.LoadInt64(&svc.Status); val != _StatusServiceStarting {
		return fmt.Errorf("Status Conflict,%d!=_StatusServiceStarting", val)
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, _StatusServiceStartFailed)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].startContainer()
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) CopyServiceConfig() (err error) {
	if val := atomic.LoadInt64(&svc.Status); val != _StatusServiceStarting {
		return fmt.Errorf("Status Conflict,%d!=_StatusServiceStarting", val)
	}
	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, _StatusServiceStartFailed)
		}

		svc.Unlock()
	}()

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

func (svc *Service) StartService() (err error) {
	if val := atomic.LoadInt64(&svc.Status); val != _StatusServiceStarting {
		return fmt.Errorf("Status Conflict,%d!=_StatusServiceStarting", val)
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, _StatusServiceStartFailed)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].startService()
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
	err := svc.removeContainers(force, rmVolumes)
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

func (svc *Service) CreateUsers() (err error) {
	svc.Lock()
	defer svc.Unlock()

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

	for i := range svc.units {
		u := svc.units[i]

		if u.Type == _SwitchManagerType {
			// create proxy users
		}
	}

	return nil
}

func (svc *Service) RefreshTopology() error {
	svc.RLock()
	defer svc.RUnlock()

	for i := range svc.units {
		u := svc.units[i]

		if u.Type == _SwitchManagerType {

			// lock

			// topology

		}
	}

	return nil
}

func (svc *Service) InitTopology() error {
	svc.RLock()
	defer svc.RUnlock()

	for i := range svc.units {
		u := svc.units[i]

		if u.Type == _SwitchManagerType {

			// lock

			// topology

		}
	}

	return nil
}

func (svc *Service) RegisterServices() (err error) {
	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {

		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].RegisterHealthCheck(nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) DeregisterServices() (err error) {
	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].DeregisterHealthCheck(nil)
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

	u, err := svc.getUnitByType("Switch Manager")

	svc.RUnlock()

	return u, err
}

func (svc *Service) getSwithManagerAddr() (addr string, port int, err error) {

	u, err := svc.getUnitByType("Switch Manager")
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
	return svc.task
}

func (gd *Gardener) DeleteService(name string, force, volumes bool, timeout int) error {
	svc, err := gd.GetService(name)
	if err != nil {
		if err == ErrServiceNotFound {
			return nil
		}

		return err
	}

	err = svc.Delete(force, volumes, timeout)
	if err != nil {
		return err
	}

	err = gd.RemoveCronJob(svc.backup.ID)

	return err

}

func (svc *Service) Delete(force, volumes bool, timeout int) error {
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

	err = svc.DeregisterServices()

	return err
}
