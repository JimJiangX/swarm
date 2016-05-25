package swarm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/yiduoyunQ/sm/sm-svr/consts"
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

func (gd *Gardener) CreateService(req structs.PostServiceRequest) (_ *Service, err error) {
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		logrus.Error("get Registry Auth Config", err)
		return nil, err
	}

	svc, err := BuildService(req, authConfig)
	if err != nil {
		logrus.Error("Build Service", err)
		return nil, err
	}

	defer func() {
		if err != nil {
			logrus.WithField("Service Name", svc.Name).Errorf("Servcie Cleaned,%v", err)

			if svc.backup != nil && svc.backup.ID != "" {
				gd.RemoveCronJob(svc.backup.ID)
			}
			sys, err := database.GetSystemConfig()
			if err != nil {
				return
			}
			clients, err := sys.GetConsulClient()
			if err != nil || len(clients) == 0 {
				return
			}

			horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
			svc.Delete(clients[0], horus, true, true, 0)
		}
	}()

	logrus.WithFields(logrus.Fields{
		"Servcie Name": svc.Name,
		"Service ID":   svc.ID,
		"Task ID":      svc.task.ID,
	}).Info("Service Saved Into Database")

	svc.failureRetry = gd.createRetry

	if err := gd.AddService(svc); err != nil {
		logrus.WithField("Service Name", svc.Name).Errorf("Service Add to Gardener Error:%s", err.Error())

		return svc, err
	}

	if svc.backup != nil {
		bs := NewBackupJob(gd.host, svc)
		err := gd.RegisterBackupStrategy(bs)
		if err != nil {
			logrus.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err.Error())
		}
	}

	if err := gd.ServiceToScheduler(svc); err != nil {
		logrus.Error("Service Add To Scheduler", err)
		return svc, err
	}
	logrus.Debugf("[mg] ServiceToScheduler ok:%v", svc)

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
	for _, u := range svc.units {
		err := u.initService()
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
	err = svc.stopService()
	svc.Unlock()

	return err
}

func (svc *Service) stopService() error {
	for _, u := range svc.units {
		err := u.stopService()
		if err != nil {
			// return err
			logrus.Errorf("container %s stop service error:%s", u.Name, err.Error())
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
			logrus.Errorf("container %s remove,-f=%b -v=%b,error:%s", u.Name, force, rmVolumes, err.Error())

			return err
		}
	}

	return nil
}

func (svc *Service) refreshTopology() error {
	svc.getSwithManagerUnit()

	return nil
}

func (svc *Service) initTopology() error {
	swm, err := svc.getSwithManagerUnit()
	if err != nil {
		return err
	}

	addr, port, err := swm.getNetworkingAddr(_ContainersNetworking, "Port")
	if err != nil {
		return err
	}
	if swm.engine != nil {
		addr = swm.engine.IP
	}

	proxys, err := svc.getUnitByType(_ProxyType)
	if err != nil {
		return err
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

	sqls, err := svc.getUnitByType(_UpsqlType)
	if err != nil {
		return err
	}
	arch := consts.Type_M
	if len(sqls) == 1 {
		arch = consts.Type_M
	} else if len(sqls) == 2 {
		arch = consts.Type_M_SB
	} else if len(sqls) > 2 {
		arch = consts.Type_M_SB_SL
	}

	dataNodes := make(map[string]swm_structs.DatabaseInfo, len(sqls))
	for i := range sqls {
		ip, dataPort, err := swm.getNetworkingAddr(_ContainersNetworking, "mysqld::port")
		if err != nil {
			return err
		}
		dataNodes[sqls[i].ID] = swm_structs.DatabaseInfo{
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

	return nil
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

func (svc *Service) DeregisterServices(client *consulapi.Client) error {
	if client == nil {
		return fmt.Errorf("consul client is nil")
	}
	svc.Lock()

	for _, u := range svc.units {
		err := u.DeregisterHealthCheck(client)
		if err != nil {
			svc.Unlock()
			return err
		}
	}
	svc.Unlock()

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

func (svc *Service) getUnitByType(Type string) ([]*unit, error) {
	units := make([]*unit, 0, len(svc.units))
	for _, u := range svc.units {
		if u.Type == Type {
			units = append(units, u)
		}
	}
	if len(units) > 0 {
		return units, nil
	}

	return nil, fmt.Errorf("Not Found unit %s In Service %s", Type, svc.ID)
}

func (svc *Service) getSwithManagerUnit() (*unit, error) {
	units, err := svc.getUnitByType(_UnitRole_SwitchManager)

	if err != nil || len(units) != 1 {
		return nil, fmt.Errorf("Unexpected num about %s unit,got %d,error:%v", _UnitRole_SwitchManager, len(units), err)
	}

	return units[0], nil
}

func (svc *Service) GetSwitchManagerAndMaster() (string, int, *unit, error) {
	svc.RLock()
	defer svc.RUnlock()

	swm, err := svc.getSwithManagerUnit()
	if err != nil {
		return "", 0, nil, err
	}

	addr, port, err := swm.getNetworkingAddr(_ContainersNetworking, "Port")
	if err != nil {
		return "", 0, nil, err
	}
	if swm.engine != nil {
		addr = swm.engine.IP
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

	sys, err := database.GetSystemConfig()
	if err != nil {
		return err
	}
	clients, err := sys.GetConsulClient()
	if err != nil || len(clients) == 0 {
		return fmt.Errorf("GetConsulClient error %v %v", err, sys)
	}

	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	err = svc.Delete(clients[0], horus, force, volumes, timeout)
	if err != nil {
		return err
	}

	// TODO: delete database records

	err = gd.RemoveCronJob(svc.backup.ID)
	if err != nil {
		return err
	}

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
		return err
	}

	err := svc.stopContainers(timeout)
	if err != nil {
		return err
	}

	err = svc.removeContainers(force, volumes)
	if err != nil {
		return err
	}

	err = svc.deregisterInHorus(horus)
	if err != nil {
		logrus.Error("%s deregister In Horus error:%s", svc.Name, err.Error())
	}

	err = svc.DeregisterServices(client)
	if err != nil {
		logrus.Error("%s deregister In consul error:%s", svc.Name, err.Error())
	}

	return nil
}
