package swarm

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	consulapi "github.com/hashicorp/consul/api"
	swm_structs "github.com/yiduoyunQ/sm/sm-svr/structs"
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
		logrus.Errorf("JSON Marshal Error:%s", err.Error())
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
		logrus.Error("Parse Service.Architecture", err)
		return nil, err
	}

	service := NewService(svc, nodeNum)

	service.Lock()
	defer service.Unlock()

	task := database.NewTask("service", svc.ID, "create service", nil, 0)
	service.task = &task

	service.backup = strategy
	service.base = &req
	service.authConfig = authConfig
	service.users = converteToUsers(service.ID, req.Users)
	atomic.StoreInt64(&svc.Status, _StatusServcieBuilding)

	if err := service.SaveToDB(); err != nil {
		atomic.StoreInt64(&svc.Status, _StatusServiceInit)

		logrus.Error("Service Save To DB", err)
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
		logrus.Error("Parse Request.BackupStrategy.Valid to time.Time", err)
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

	_, err := database.GetService(req.Name)
	if err == nil {
		warnings = append(warnings, fmt.Sprintf("Service Name %s exist", req.Name))
	}

	arch, _, err := getServiceArch(req.Architecture)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Parse 'Architecture' Failed,%s", err.Error()))
	}

	for _, module := range req.Modules {
		if _, _, err := initialize(module.Type); err != nil {
			warnings = append(warnings, err.Error())
		}

		//if !isStringExist(module.Type, supportedServiceTypes) {
		//	warnings = append(warnings, fmt.Sprintf("Unsupported '%s' Yet", module.Type))
		//}
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

	logrus.Warnf("Service Valid warning:", warnings)

	return warnings
}

func ValidateServiceScaleUp(svc *Service, list []structs.ScaleUpModule) error {
	warns := make([]string, 0, 10)

	for i := range list {
		_, _, err := initialize(list[i].Type)
		if err != nil {
			warns = append(warns, err.Error())
		}

		err = validateContainerUpdateConfig(list[i].Config)
		if err != nil {
			warns = append(warns, err.Error())
		}
	}

	svc.RLock()
	for i := range list {
		units := svc.getUnitByType(list[i].Type)
		if len(units) == 0 {
			warns = append(warns, fmt.Sprintf("Not Found unit '%s' In Service %s", list[i].Type, svc.Name))
		}
		for _, u := range units {
			if u.engine == nil || (u.config == nil && u.container == nil) {
				warns = append(warns, fmt.Sprintf("unit odd,%+v", u))
			}
		}
	}
	svc.RUnlock()

	if len(warns) == 0 {
		return nil
	}

	return fmt.Errorf("Warnings:%s", warns)
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

	err = database.InsertBackupStrategy(*backup)
	if err != nil {
		return nil, fmt.Errorf("Insert Backup Strategy Error:%s", err.Error())
	}

	svc.Lock()
	svc.backup = backup
	svc.Unlock()

	return backup, nil
}

func (svc *Service) UpdateBackupStrategy(backup database.BackupStrategy) error {
	err := database.UpdateBackupStrategy(backup)
	if err != nil {
		return fmt.Errorf("Insert Backup Strategy Error:%s", err.Error())
	}

	svc.Lock()
	svc.backup = &backup
	svc.Unlock()

	return nil

}

func DeleteServiceBackupStrategy(strategy string) error {
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
	list := make([]database.User, 0, len(users)+5)
	now := time.Now()
	for i := range users {
		if users[i].Type != _User_Type_Proxy &&
			users[i].Role != _User_DB &&
			users[i].Role != _User_Application {
			continue
		}

		list = append(list, database.User{
			ID:         utils.Generate32UUID(),
			ServiceID:  service,
			Type:       users[i].Type,
			Username:   users[i].Username,
			Password:   users[i].Password,
			Role:       users[i].Role,
			Permission: "",
			CreatedAt:  now,
		})
	}

	sys, err := database.GetSystemConfig()
	if err != nil {
		logrus.Error(err)
	}

	list = append(list,
		database.User{
			ID:         utils.Generate32UUID(),
			ServiceID:  service,
			Type:       _User_Type_DB,
			Username:   sys.MonitorUsername,
			Password:   sys.MonitorPassword,
			Role:       _User_Monitor,
			Permission: "",
			CreatedAt:  now,
		},
		database.User{
			ID:         utils.Generate32UUID(),
			ServiceID:  service,
			Type:       _User_Type_DB,
			Username:   sys.ApplicationUsername,
			Password:   sys.ApplicationPassword,
			Role:       _User_Application,
			Permission: "",
			CreatedAt:  now,
		},
		database.User{
			ID:         utils.Generate32UUID(),
			ServiceID:  service,
			Type:       _User_Type_DB,
			Username:   sys.DBAUsername,
			Password:   sys.DBAPassword,
			Role:       _User_DBA,
			Permission: "",
			CreatedAt:  now,
		},
		database.User{
			ID:         utils.Generate32UUID(),
			ServiceID:  service,
			Type:       _User_Type_DB,
			Username:   sys.DBUsername,
			Password:   sys.DBPassword,
			Role:       _User_DB,
			Permission: "",
			CreatedAt:  now,
		},
		database.User{
			ID:         utils.Generate32UUID(),
			ServiceID:  service,
			Type:       _User_Type_DB,
			Username:   sys.ReplicationUsername,
			Password:   sys.ReplicationPassword,
			Role:       _User_Replication,
			Permission: "",
			CreatedAt:  now,
		})

	return list
}
func (svc *Service) getUnit(IDOrName string) (*unit, error) {
	for _, u := range svc.units {
		if u.ID == IDOrName || u.Name == IDOrName {
			return u, nil
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
	return database.TxSaveService(svc.Service, svc.backup, svc.task, svc.users)
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
			logrus.Warnf("JSON Unmarshal Service.Description Error:%s,Description:%s", err.Error(), service.Description)
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
		logrus.Errorf("Registry Auth Config Error:%s", err.Error())
		return nil, err
	}

	svc := NewService(service, len(units))
	svc.Lock()
	defer svc.Unlock()

	for i := range units {
		// rebuild units
		u, err := gd.rebuildUnit(units[i])
		if err != nil {
			return nil, err
		}

		svc.units = append(svc.units, &u)
	}
	svc.backup = backup
	svc.base = base
	svc.authConfig = authConfig

	gd.Lock()
	gd.services = append(gd.services, svc)
	gd.Unlock()

	return svc, nil

}

func (gd *Gardener) CreateService(req structs.PostServiceRequest) (*Service, *database.BackupStrategy, error) {
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		logrus.Error("get Registry Auth Config", err)
		return nil, nil, err
	}

	svc, err := BuildService(req, authConfig)
	if err != nil {
		logrus.Error("Build Service", err)
		return nil, nil, err
	}

	defer func() {
		if err != nil {
			logrus.WithField("Service Name", svc.Name).Errorf("Servcie Cleaned,%v", err)
			gd.RemoveService(svc.Name, true, true, 0)
		}
	}()

	logrus.WithFields(logrus.Fields{
		"Servcie Name": svc.Name,
		"Service ID":   svc.ID,
		"Task ID":      svc.task.ID,
	}).Info("Service Saved Into Database")

	svc.failureRetry = gd.createRetry

	err = gd.AddService(svc)
	if err != nil {
		logrus.WithField("Service Name", svc.Name).Errorf("Service Add to Gardener Error:%s", err.Error())

		return svc, nil, err
	}

	svc.RLock()
	defer svc.RUnlock()

	if svc.backup != nil {
		bs := NewBackupJob(gd.host, svc)
		err = gd.RegisterBackupStrategy(bs)
		if err != nil {
			logrus.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err.Error())
		}
	}

	err = gd.ServiceToScheduler(svc)
	if err != nil {
		logrus.Error("Service Add To Scheduler", err)
		return svc, svc.backup, err
	}
	logrus.Debugf("[mg] ServiceToScheduler ok:%v", svc)

	return svc, svc.backup, nil
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
	for _, u := range svc.units {
		err := u.startContainer()
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) copyServiceConfig() error {
	for _, u := range svc.units {
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
	wg := new(sync.WaitGroup)
	errCh := make(chan error, 5)
	for _, u := range svc.units {
		wg.Add(1)

		go func(u *unit, wg *sync.WaitGroup, ch chan error) {
			defer wg.Done()

			err := u.initService()
			if err != nil {
				logrus.Errorf("container %s init service error:%s", u.Name, err)
				ch <- err
			}
		}(u, wg, errCh)
	}

	wg.Wait()
	close(errCh)

	var err error = nil
	for err1 := range errCh {
		if err1 != nil {
			err = err1
		}
	}

	return err
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
	for _, u := range svc.units {
		err := u.startService()
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) StopContainers(timeout int) error {
	err := svc.checkStatus(_StatusServiceNoContent)
	if err != nil {
		return err
	}

	svc.Lock()
	err = svc.stopContainers(timeout)
	svc.Unlock()

	return err
}

func (svc *Service) stopContainers(timeout int) error {
	for _, u := range svc.units {
		err := u.stopContainer(timeout)
		if err != nil {
			logrus.Errorf("container %s stop error:%s", u.Name, err.Error())
			return err
		}
	}

	return nil
}
func (svc *Service) StopService() error {
	err := svc.checkStatus(_StatusServiceNoContent)
	if err != nil {
		return err
	}

	svc.Lock()
	defer svc.Unlock()

	addr, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		logrus.Warnf("service %s get SwithManager Addr error:%s", svc.Name, err)
	} else {
		err = smlib.Lock(addr, port)
		if err != nil {

		}
	}

	units := svc.getUnitByType(_UpsqlType)
	if len(units) == 0 {
		err = fmt.Errorf("Not Found unit by type %s In Service %s", _UpsqlType, svc.Name)
		logrus.Error(err)

		return err
	}

	for _, u := range units {
		err = u.stopService()
		if err != nil {
			logrus.Errorf("service %s unit %s stop service error:%s", svc.Name, u.Name, err)
			return err
		}
	}

	return nil
}

func (svc *Service) stopService() error {
	for _, u := range svc.units {
		err := u.stopService()
		if err != nil {
			logrus.Errorf("container %s stop service error:%s", u.Name, err.Error())
			// return err
		}
	}

	return nil
}

func (svc *Service) RemoveContainers(force, rmVolumes bool) error {
	if !force {
		err := svc.checkStatus(_StatusServiceNoContent)
		if err != nil {
			return err
		}
	}

	svc.Lock()

	err := svc.stopService()
	if err != nil {
		svc.Unlock()

		return err
	}

	err = svc.removeContainers(force, rmVolumes)

	svc.Unlock()

	return err
}

func (svc *Service) removeContainers(force, rmVolumes bool) error {
	for _, u := range svc.units {
		err := u.removeContainer(force, rmVolumes)
		if err != nil {
			logrus.Errorf("container %s remove,-f=%v -v=%v,error:%s", u.Name, force, rmVolumes, err.Error())

			return err
		}
		logrus.Debugf("container %s removed", u.Name)
	}

	return nil
}

func (svc *Service) refreshTopology() error {
	svc.getSwithManagerUnit()

	return nil
}

func (svc *Service) initTopology() error {
	swm := svc.getSwithManagerUnit()
	sqls := svc.getUnitByType(_UpsqlType)
	proxys := svc.getUnitByType(_ProxyType)
	if len(proxys) == 0 || len(sqls) == 0 || swm == nil {
		return nil
	}
	addr, port, err := swm.getNetworkingAddr(_ContainersNetworking, "Port")
	if err != nil {
		return err
	}
	if swm.engine != nil {
		addr = swm.engine.IP
	}

	proxyNames := make([]string, len(proxys))
	for i := range proxys {
		proxyNames[i] = proxys[i].ID
	}

	users := make([]swm_structs.User, len(svc.users))
	for i := range svc.users {
		users[i] = swm_structs.User{
			Id:       svc.users[i].ID,
			Type:     svc.users[i].Type,
			UserName: svc.users[i].Username,
			Password: svc.users[i].Password,
			Role:     svc.users[i].Role,
			ReadOnly: false,
		}
	}

	arch := _DB_Type_M
	switch num := len(sqls); {
	case num == 1:
		arch = _DB_Type_M
	case num == 2:
		arch = _DB_Type_M_SB
	case num > 2:
		arch = _DB_Type_M_SB_SL
	default:
		return fmt.Errorf("get %d units by type:%d", num, _UpsqlType)
	}

	dataNodes := make(map[string]swm_structs.DatabaseInfo, len(sqls))
	for i := range sqls {
		ip, dataPort, err := sqls[i].getNetworkingAddr(_ContainersNetworking, "mysqld::port")
		if err != nil {
			return err
		}
		dataNodes[sqls[i].Name] = swm_structs.DatabaseInfo{
			Ip:   ip,
			Port: dataPort,
		}
	}

	sys, err := database.GetSystemConfig()
	if err != nil {
		return err
	}

	topolony := swm_structs.MgmPost{
		DbaasType:           arch,                    //  string   `json:"dbaas-type"`
		DbRootUser:          sys.DBAUsername,         //  string   `json:"db-root-user"`
		DbRootPassword:      sys.DBAPassword,         //  string   `json:"db-root-password"`
		DbReplicateUser:     sys.ReplicationUsername, //  string   `json:"db-replicate-user"`
		DbReplicatePassword: sys.ReplicationPassword, //  string   `json:"db-replicate-password"`
		SwarmApiVersion:     "1.22",                  //  string   `json:"swarm-api-version,omitempty"`
		ProxyNames:          proxyNames,              //  []string `json:"proxy-names"`
		Users:               users,                   //  []User   `json:"users"`
		DataNode:            dataNodes,               //  map[string]DatabaseInfo `json:"data-node"`
	}

	err = smlib.InitSm(addr, port, topolony)
	if err != nil {
		logrus.Errorf("%s Init Topology Error %s", svc.Name, err)
	}

	return err
}

func (svc *Service) registerServices(config database.ConsulConfig) (err error) {
	for _, u := range svc.units {
		err = u.RegisterHealthCheck(config, svc)
		if err != nil {

			return err
		}
	}

	return nil
}

func (svc *Service) deregisterServices(client *consulapi.Client) error {
	if client == nil {
		return fmt.Errorf("consul client is nil")
	}

	for _, u := range svc.units {
		err := u.DeregisterHealthCheck(client)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) registerToHorus(addr, user, password string, agentPort int) error {
	params := make([]registerService, len(svc.units))

	for i, u := range svc.units {
		obj, err := u.registerHorus(user, password, agentPort)
		if err != nil {
			err = fmt.Errorf("container %s register Horus Error:%s", u.Name, err.Error())
			logrus.Error(err)

			return err
		}
		params[i] = obj
	}

	return registerToHorus(addr, params)
}

func (svc *Service) deregisterInHorus(addr string) error {
	endpoints := make([]deregisterService, len(svc.units))

	for i, u := range svc.units {
		endpoints[i] = deregisterService{Endpoint: u.ID}
	}

	return deregisterToHorus(addr, endpoints)
}

func (svc *Service) getUnitByType(Type string) []*unit {
	units := make([]*unit, 0, len(svc.units))
	for _, u := range svc.units {
		if u.Type == Type {
			units = append(units, u)
		}
	}
	if len(units) > 0 {
		return units
	}

	logrus.Warnf("Not Found unit %s In Service %s", Type, svc.Name)

	return nil
}

func (svc *Service) getSwithManagerUnit() *unit {
	units := svc.getUnitByType(_UnitRole_SwitchManager)
	if len(units) != 1 {
		logrus.Warnf("Unexpected num about %s unit,got %d", _UnitRole_SwitchManager, len(units))
		return nil
	}

	return units[0]
}

func (svc *Service) getSwitchManagerAddr() (string, int, error) {
	swm := svc.getSwithManagerUnit()
	if swm == nil {
		return "", 0, fmt.Errorf("Not Found unit by type %s in service %s", _UnitRole_SwitchManager, svc.Name)
	}

	addr, port, err := swm.getNetworkingAddr(_ContainersNetworking, "Port")
	if err != nil {
		return "", 0, err
	}
	if swm.engine != nil {
		addr = swm.engine.IP
	}

	return addr, port, nil
}

func (svc *Service) GetSwitchManagerAndMaster() (string, int, *unit, error) {
	svc.RLock()
	defer svc.RUnlock()

	addr, port, err := svc.getSwitchManagerAddr()
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
		if err != nil || master == nil {
			if retries > 0 {
				continue
			}
			err1 := database.UpdateTaskStatus(&task, _StatusTaskCancel, time.Now(), "Cancel,The Task marked as TaskCancel,"+err.Error())
			return fmt.Errorf("Errors:%v,%v", err, err1)
		}

		if err := smlib.Lock(addr, port); err != nil {
			if retries > 0 {
				continue
			}
			err1 := database.UpdateTaskStatus(&task, _StatusTaskCancel, time.Now(), "TaskCancel,Switch Manager is busy now,"+err.Error())
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

		err = database.UpdateTaskStatus(&task, _StatusTaskTimeout, time.Now(), "Timeout,The Task marked as TaskTimeout")

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

type pendingContainerUpdate struct {
	containerID string
	cpusetCPus  string
	unit        *unit
	svc         *Service
	engine      *cluster.Engine
	config      container.UpdateConfig
}

func (gd *Gardener) ServiceScaleUp(name string, list []structs.ScaleUpModule) error {
	svc, err := gd.GetService(name)
	if err != nil {
		return err
	}

	err = ValidateServiceScaleUp(svc, list)
	if err != nil {
		return err
	}

	svc.Lock()
	gd.scheduler.Lock()
	defer func() {
		svc.Unlock()
		gd.scheduler.Unlock()
	}()

	pendings := make([]pendingContainerUpdate, 0, len(svc.units))

	for i := range list {
		err := handlePerScaleUpModule(gd, svc, list[i], &pendings)
		if err != nil {
			return err
		}
	}

	for i := range pendings {
		if pendings[i].svc == nil {
			pendings[i].svc = svc
		}
		err := pendings[i].containerUpdate()
		if err != nil {
			logrus.Error("container %s update error:%s", pendings[i].containerID, err)
			return err
		}
	}

	return nil
}

func handlePerScaleUpModule(gd *Gardener, svc *Service, module structs.ScaleUpModule, pendings *[]pendingContainerUpdate) error {
	need, err := strconv.ParseInt(module.Config.CpusetCpus, 10, 64)
	if err != nil {
		if module.Config.CpusetCpus == "" {
			need = 0
		} else {
			return err
		}
	}

	units := svc.getUnitByType(module.Type)
	if len(units) == 0 {
		return fmt.Errorf("Not Found unit '%s' In Service %s", module.Type, svc.Name)
	}

	var used int64
	if need > 0 {
		used, err = utils.GetCPUNum(units[0].container.Info.HostConfig.CpusetCpus)
		if err != nil {
			return err
		}
	}
	if (need == 0 || used == need) && (module.Config.Memory == 0 ||
		module.Config.Memory == units[0].container.Info.HostConfig.Memory) {
		return nil
	}

	for _, u := range units {
		if u.engine.Memory-u.engine.UsedMemory()-int64(module.Config.Memory)+u.container.Config.HostConfig.Memory < 0 {
			return fmt.Errorf("Engine %s:%s have not enough Memory for Container %s Update", u.engine.ID, u.engine.IP, u.Name)
		}
	}

	if need == used || need == 0 {
		for _, u := range units {
			*pendings = append(*pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				unit:        u,
				engine:      u.engine,
				config:      module.Config,
			})
		}
	} else if need < used {
		for _, u := range units {
			cpusetCpus, err := reduceCPUset(u.container.Info.HostConfig.CpusetCpus, int(need))
			if err != nil {
				return err
			}
			*pendings = append(*pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				cpusetCPus:  cpusetCpus,
				unit:        u,
				engine:      u.engine,
				config:      module.Config,
			})
		}
	} else {
		for _, u := range units {
			reserve := make([]string, 0, len(svc.units))
			for _, pending := range *pendings {
				if u.engine.ID == pending.engine.ID {
					reserve = append(reserve, pending.cpusetCPus)
				}
			}
			cpusetCpus, err := gd.allocCPUs(u.engine, int(need-used), reserve...)
			if err != nil {
				return err
			}
			cpusetCpus = u.container.Info.HostConfig.CpusetCpus + "," + cpusetCpus
			*pendings = append(*pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				cpusetCPus:  cpusetCpus,
				unit:        u,
				engine:      u.engine,
				config:      module.Config,
			})
		}
	}

	return nil
}

func (p *pendingContainerUpdate) containerUpdate() error {
	if p.cpusetCPus != "" && p.config.CpusetCpus != "" {
		p.config.CpusetCpus = p.cpusetCPus
	}

	err := p.unit.stopService()
	if err != nil {
		logrus.Warn("container %s stop service error:%s", p.unit.Name, err)
	}

	err = p.unit.updateContainer(p.config)
	if err != nil {
		return err
	}

	defConfig, err := p.unit.defaultUserConfig(p.svc, p.unit)
	if err != nil {
		return err
	}
	err = p.unit.CopyConfig(defConfig)
	if err != nil {
		return err
	}

	err = p.unit.startService()
	if err != nil {
		return err
	}

	return nil
}

func reduceCPUset(cpusetCpus string, need int) (string, error) {
	cpus, err := utils.ParseUintList(cpusetCpus)
	if err != nil {
		return "", err
	}

	cpuSlice := make([]int, 0, len(cpus))
	for k, ok := range cpus {
		if ok {
			cpuSlice = append(cpuSlice, k)
		}
	}
	sort.Ints(cpuSlice)

	cpuString := make([]string, need)
	for n := 0; n < need; n++ {
		cpuString[n] = strconv.Itoa(cpuSlice[n])
	}

	return strings.Join(cpuString, ","), nil
}

func (gd *Gardener) RemoveService(name string, force, volumes bool, timeout int) error {
	entry := logrus.WithFields(logrus.Fields{
		"Name":    name,
		"force":   force,
		"volumes": volumes,
	})
	entry.Info("Removing Service...")

	service, err := database.GetService(name)
	if err != nil {
		entry.Errorf("GetService From DB error:%s", err)

		return nil
	}
	entry.Infof("Found Service %s:%s", service.Name, service.ID)

	entry.Debug("GetService From Gardener...")
	svc, err := gd.GetService(service.ID)
	if err != nil {
		if err == ErrServiceNotFound {
			return nil
		}
		entry.Errorf("GetService From Gardener error:%s", err)

		return err
	}

	entry.Debug("Prepare params,GetSystemConfig...")
	sys, err := database.GetSystemConfig()
	if err != nil {
		entry.Errorf("GetSystemConfig error:%s", err)
		return err
	}
	clients, err := sys.GetConsulClient()
	if err != nil || len(clients) == 0 {
		return fmt.Errorf("GetConsulClient error %v %v", err, sys)
	}

	entry.Debug("Service Delete... stop service & stop containers & rm containers & deregister")
	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	err = svc.Delete(clients[0], horus, force, volumes, timeout)
	if err != nil {
		entry.Errorf("Service.Delete error:%s", err)

		return err
	}

	// delete database records relation svc.ID
	entry.Debug("DeteleServiceRelation...")
	err = database.DeteleServiceRelation(svc.ID)
	if err != nil {
		entry.Errorf("DeteleServiceRelation error:%s", err)

		return err
	}

	if svc.backup != nil {
		err = gd.RemoveCronJob(svc.backup.ID)
		if err != nil {
			entry.Errorf("RemoveCronJob %s error:%s", svc.backup.ID, err)

			return err
		}
	}

	entry.Debug("Remove Service From Gardener...")
	gd.Lock()
	for i := range gd.services {
		if gd.services[i].ID == name || gd.services[i].Name == name {
			gd.services = append(gd.services[:i], gd.services[i+1:]...)
			break
		}
	}
	gd.Unlock()

	return nil
}

func (svc *Service) Delete(client *consulapi.Client, horus string, force, volumes bool, timeout int) error {
	svc.Lock()
	defer svc.Unlock()

	if err := svc.stopService(); err != nil {
		logrus.Error("%s stop Service error:%s", svc.Name, err.Error())
		// return err
	}

	err := svc.stopContainers(timeout)
	if err != nil {
		// return err
	}

	err = svc.removeContainers(force, volumes)
	if err != nil {
		return err
	}

	err = svc.deregisterInHorus(horus)
	if err != nil {
		logrus.Errorf("%s deregister In Horus error:%s", svc.Name, err.Error())
	}

	err = svc.deregisterServices(client)
	if err != nil {
		logrus.Errorf("%s deregister In consul error:%s", svc.Name, err.Error())
	}

	return nil
}
