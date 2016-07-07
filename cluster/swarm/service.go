package swarm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
		logrus.Errorf("JSON Marshal Error:%s", err)
		return nil, err
	}

	svc := database.Service{
		ID:                   utils.Generate64UUID(),
		Name:                 req.Name,
		Description:          string(des),
		Architecture:         req.Architecture,
		BusinessCode:         req.BusinessCode,
		AutoHealing:          req.AutoHealing,
		AutoScaling:          req.AutoScaling,
		HighAvailable:        req.HighAvailable,
		Status:               _StatusServiceInit,
		BackupMaxSizeByte:    req.BackupMaxSize,
		BackupFilesRetention: req.BackupRetention,
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
	sys, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}
	users := defaultServiceUsers(svc.ID, *sys)
	users = append(users, converteToUsers(svc.ID, req.Users)...)

	service := NewService(svc, nodeNum)

	service.Lock()
	defer service.Unlock()

	task := database.NewTask("service", svc.ID, "create service", nil, 0)
	service.task = &task

	service.backup = strategy
	service.base = &req
	service.authConfig = authConfig
	service.users = users
	atomic.StoreInt64(&svc.Status, _StatusServcieBuilding)

	if err := service.SaveToDB(); err != nil {
		atomic.StoreInt64(&svc.Status, _StatusServiceInit)

		logrus.Errorf("Service Save To DB %s", err)
		return nil, err
	}

	return service, nil
}

func newBackupStrategy(service string, strategy *structs.BackupStrategy) (*database.BackupStrategy, error) {
	if strategy == nil {
		return nil, nil
	}
	var (
		valid = time.Time{}
		err   error
	)
	if strategy.Valid != "" {
		valid, err = utils.ParseStringToTime(strategy.Valid)
		if err != nil {
			logrus.Error("Parse Request.BackupStrategy.Valid to time.Time", err)
			return nil, err
		}
	}

	return &database.BackupStrategy{
		ID:        utils.Generate64UUID(),
		Name:      strategy.Name,
		Type:      strategy.Type,
		ServiceID: service,
		Spec:      strategy.Spec,
		Valid:     valid,
		Enabled:   true,
		BackupDir: strategy.BackupDir,
		Timeout:   strategy.Timeout,
		CreatedAt: time.Now(),
	}, nil
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
		return nil, fmt.Errorf("Insert Backup Strategy Error:%s", err)
	}

	svc.Lock()
	svc.backup = backup
	svc.Unlock()

	return backup, nil
}

func (svc *Service) UpdateBackupStrategy(backup database.BackupStrategy) error {
	err := database.UpdateBackupStrategy(backup)
	if err != nil {
		return fmt.Errorf("Insert Backup Strategy Error:%s", err)
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

func (svc *Service) AddServiceUsers(req []structs.User) (int, error) {
	svc.Lock()
	defer svc.Unlock()

	code := 200

	if len(svc.users) == 0 {
		out, err := database.ListUsersByService(svc.ID, "")
		if err != nil {
			return 0, err
		}

		svc.users = out
	}

	update := make([]database.User, 0, len(req))
	addition := make([]structs.User, 0, len(req))
	for i := range req {
		exist := false
		for u := range svc.users {
			if svc.users[u].Username == req[i].Username {
				update = append(update, database.User{
					ID:        svc.users[u].ID,
					ServiceID: svc.users[u].ServiceID,
					Type:      req[i].Type,
					Username:  req[i].Username,
					Password:  req[i].Password,
					Role:      req[i].Role,
				})
				exist = true
				break
			}
		}
		if !exist {
			addition = append(addition, req[i])
		}
	}

	addr, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		return 0, err
	}

	users := converteToUsers(svc.ID, addition)
	for i := range users {
		code = 201
		user := swm_structs.User{
			Id:       users[i].ID,
			Type:     users[i].Type,
			UserName: users[i].Username,
			Password: users[i].Password,
			Role:     users[i].Role,
		}
		err := smlib.AddUser(addr, port, user)
		if err != nil {
			logrus.Errorf("%s add user error:%s", addr, err)
			return 0, err
		}
	}

	for i := range update {
		user := swm_structs.User{
			Id:       update[i].ID,
			Type:     update[i].Type,
			UserName: update[i].Username,
			Password: update[i].Password,
			Role:     update[i].Role,
		}
		err := smlib.UptUser(addr, port, user)
		if err != nil {
			logrus.Errorf("%s update user error:%s", addr, err)
			return 0, err
		}
	}

	err = database.TxUpdateUsers(users, update)
	if err != nil {
		return 0, err
	}

	out, err := database.ListUsersByService(svc.ID, "")
	if err != nil {
		return 0, err
	}

	svc.users = out

	return code, nil
}

func (svc *Service) DeleteServiceUsers(usernames []string, all bool) error {
	svc.Lock()
	defer svc.Unlock()

	if len(svc.users) == 0 {
		out, err := database.ListUsersByService(svc.ID, "")
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return fmt.Errorf("'%s' are not Service '%s' Usernames", usernames, svc.Name)
		}
		svc.users = out
	}

	list := make([]database.User, 0, len(usernames))
	if all {
		list = svc.users
	} else {
		none := make([]string, 0, len(usernames))
		for i := range usernames {
			exist := false
			for u := range svc.users {
				if svc.users[u].Username == usernames[i] {
					list = append(list, svc.users[u])
					exist = true
					break
				}
			}
			if !exist {
				none = append(none, usernames[i])
			}
		}

		if len(none) > 0 {
			return fmt.Errorf("'%s' are not Service '%s' Usernames", none, svc.Name)
		}
	}

	addr, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		return err
	}
	for i := range list {
		user := swm_structs.User{
			Id:       list[i].ID,
			Type:     list[i].Type,
			UserName: list[i].Username,
			Password: list[i].Password,
			Role:     list[i].Role,
		}
		err := smlib.DelUser(addr, port, user)
		if err != nil {
			return err
		}
	}
	err = database.TxDeleteUsers(list)
	if err != nil {
		return err
	}

	out, err := database.ListUsersByService(svc.ID, "")
	if err != nil {
		return err
	}

	svc.users = out

	return nil
}

func defaultServiceUsers(service string, sys database.Configurations) []database.User {
	now := time.Now()
	return []database.User{
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
		},
	}
}

func converteToUsers(service string, users []structs.User) []database.User {
	list := make([]database.User, len(users))
	now := time.Now()
	for i := range users {
		if users[i].Type != _User_Type_Proxy &&
			users[i].Role != _User_DB &&
			users[i].Role != _User_Application {
			continue
		}

		list[i] = database.User{
			ID:         utils.Generate32UUID(),
			ServiceID:  service,
			Type:       users[i].Type,
			Username:   users[i].Username,
			Password:   users[i].Password,
			Role:       users[i].Role,
			Permission: "",
			CreatedAt:  now,
		}
	}

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
			logrus.Warnf("JSON Unmarshal Service.Description Error:%s,Description:%s", err, service.Description)
		}
	}

	var backup *database.BackupStrategy
	strategies, err := database.ListBackupStrategyByServiceID(service.ID)
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
		logrus.Errorf("Registry Auth Config Error:%s", err)
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

func (gd *Gardener) CreateService(req structs.PostServiceRequest) (_ *Service, _ *database.BackupStrategy, err error) {
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
		logrus.WithField("Service Name", svc.Name).Errorf("Service Add to Gardener Error:%s", err)

		return svc, nil, err
	}

	svc.RLock()
	defer svc.RUnlock()

	if svc.backup != nil {
		bs := NewBackupJob(svc)
		err = gd.RegisterBackupStrategy(bs)
		if err != nil {
			logrus.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err)
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

func (svc *Service) StartService() (err error) {
	err = svc.statusCAS(_StatusServiceNoContent, _StatusServiceStarting)
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
			logrus.Errorf("%s start container error:%s", u.Name, err)
			return err
		}
	}

	return nil
}

func (svc *Service) copyServiceConfig() error {
	for _, u := range svc.units {
		forbid, can := u.CanModify(u.configures)
		if !can {
			return fmt.Errorf("Forbid modifying service configs,%s", forbid)
		}
		defConfig, err := u.defaultUserConfig(svc, u)
		if err != nil {
			return err
		}

		for key, val := range u.configures {
			defConfig[key] = val
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
			logrus.Errorf("container %s stop error:%s", u.Name, err)
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

	// TODO:is this necessary?
	addr, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		logrus.Warnf("service %s get SwithManager Addr error:%s", svc.Name, err)
	} else {
		err = smlib.Lock(addr, port)
		if err != nil {
			logrus.Error(err)
		} else {
			defer smlib.UnLock(addr, port)
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
			logrus.Errorf("container %s stop service error:%s", u.Name, err)

			err1 := checkContainerError(err)
			if err1 == errContainerNotRunning || err1 == errContainerNotFound {
				continue
			}
			return err
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
	logrus.Debug(svc.Name, " remove Containers")
	for _, u := range svc.units {
		logrus.Debug(u.Name, " remove Container")
		err := u.removeContainer(force, rmVolumes)
		if err != nil {
			logrus.Errorf("container %s remove,-f=%v -v=%v,error:%s", u.Name, force, rmVolumes, err)

			if err := checkContainerError(err); err == errContainerNotFound {
				continue
			}

			return err
		}
		logrus.Debugf("container %s removed", u.Name)
	}

	return nil
}

func (svc *Service) UpdateUnitConfig(_type string, config map[string]interface{}) error {
	svc.Lock()
	defer svc.Unlock()

	for key, val := range config {
		delete(config, key)
		config[strings.ToLower(key)] = val
	}

	units := svc.getUnitByType(_type)

	for _, u := range units {
		keys, ok := u.CanModify(config)
		if !ok {
			return fmt.Errorf("Illegal keys:%s,Or keys cannot be modified", keys)
		}

		err := u.CopyConfig(config)
		if err != nil {
			return err
		}

		if u.MustRestart(config) {
			err := u.stopService()
			if err != nil {
				logrus.Error("%s stop Service error,%s", u.Name, err)
			}

			err = u.startService()
			if err != nil {
				logrus.Error("%s start Service error,%s", u.Name, err)
				return err
			}

		}
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
		err = registerHealthCheck(u, config, svc)
		if err != nil {

			return err
		}
	}

	return nil
}

func (svc *Service) deregisterServices(config consulapi.Config) error {
	for _, u := range svc.units {
		eng, err := u.getEngine()
		if err != nil {
			logrus.Error(err)
			continue
		}
		err = deregisterHealthCheck(eng.IP, u.ID, config)
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
			err = fmt.Errorf("container %s register Horus Error:%s", u.Name, err)
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

func (svc *Service) getUnitByType(_type string) []*unit {
	units := make([]*unit, 0, len(svc.units))
	for _, u := range svc.units {
		if u.Type == _type {
			units = append(units, u)
		}
	}
	if len(units) > 0 {
		return units
	}

	logrus.Warnf("Not Found unit %s In Service %s", _type, svc.Name)

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

func (gd *Gardener) UnitIsolate(name string) error {
	table, err := database.GetUnit(name)
	if err != nil {
		logrus.Errorf("Get Unit error:%s %s", err, name)
		return err
	}

	service, err := gd.GetService(table.ServiceID)
	if err != nil {
		logrus.Errorf("Get Service error:%s %s", err, table.ServiceID)
		return err
	}

	service.Lock()
	err = service.isolate(table.Name)
	service.Unlock()

	return err
}

func (service *Service) isolate(name string) error {
	ip, port, err := service.getSwitchManagerAddr()
	if err != nil {
		logrus.Errorf("get SwitchManager Addr error:%s", err)
		return err
	}

	err = smlib.Isolate(ip, port, name)

	if err != nil {
		logrus.Errorf("Isolate %s error:%s:%d %s", name, ip, port, err)
	}

	return err
}

func (gd *Gardener) UnitSwitchBack(name string) error {
	table, err := database.GetUnit(name)
	if err != nil {
		logrus.Errorf("Get Unit error:%s %s", err, name)
		return err
	}

	service, err := gd.GetService(table.ServiceID)
	if err != nil {
		logrus.Errorf("Get Service error:%s %s", err, table.ServiceID)
		return err
	}

	service.Lock()
	err = service.switchBack(table.Name)
	service.Unlock()

	return err
}

func (service *Service) switchBack(name string) error {
	ip, port, err := service.getSwitchManagerAddr()
	if err != nil {
		logrus.Errorf("get SwitchManager Addr error:%s", err)
		return err
	}

	err = smlib.Recover(ip, port, name)
	if err != nil {
		logrus.Errorf("Recover %s error:%s:%d %s", name, ip, port, err)
	}

	return err

}

func (gd *Gardener) TemporaryServiceBackupTask(service, NameOrID string) (string, error) {
	if NameOrID != "" {
		u, err := database.GetUnit(NameOrID)
		if err != nil {
			logrus.Errorf("Not Found Unit '%s',Error:%s", NameOrID, err)
			return "", err
		}

		if service == "" {
			service = u.ServiceID
		}
	}

	svc, err := gd.GetService(service)
	if err != nil {
		logrus.Errorf("Not Found Service '%s',Error:%s", service, err)

		return "", err
	}

	var backup *unit = nil
	if NameOrID != "" {
		backup, err = svc.getUnit(NameOrID)
		if err != nil {
			return "", err
		}
	}

	addr, port, master, err := lockSwitchManager(svc, 3)
	if err != nil {
		logrus.Error(err)

		return "", err
	}
	if backup == nil {
		backup = master
	}

	sys, err := gd.SystemConfig()
	if err != nil {
		logrus.Errorf("Get SystemConfig Error:%s", err)
		return "", err
	}

	now := time.Now()
	strategy := database.BackupStrategy{
		ID:        utils.Generate64UUID(),
		Name:      backup.Name + "_backup_manually" + utils.TimeToString(now),
		Type:      "full",
		ServiceID: svc.ID,
		Spec:      "manually",
		Valid:     now,
		Enabled:   false,
		BackupDir: sys.BackupDir,
		Timeout:   24 * 60 * 60,
		CreatedAt: now,
	}

	task := database.NewTask("backup_strategy", strategy.ID, "", nil, strategy.Timeout)
	task.Status = _StatusTaskCreate
	err = database.TxInsertBackupStrategyAndTask(strategy, task)
	if err != nil {
		logrus.Errorf("TxInsert BackupStrategy And Task Erorr:%s", err)
		return "", err
	}

	go backupTask(master, &task, strategy, func() error {
		err := smlib.UnLock(addr, port)
		if err != nil {
			logrus.Errorf("switch_manager %s:%d Unlock Error:%s", addr, port, err)
		}

		return err
	})

	return task.ID, nil
}

func (svc *Service) Task() *database.Task {

	return svc.task
}

type pendingContainerUpdate struct {
	containerID string
	cpusetCpus  string
	unit        *unit
	svc         *Service
	engine      *cluster.Engine
	config      container.UpdateConfig
}

func (gd *Gardener) ServiceScaleTask(name string, scale structs.PostServiceScaledRequest) (string, error) {
	svc, err := gd.GetService(name)
	if err != nil {
		return "", err
	}

	err = ValidateServiceScale(svc, scale)
	if err != nil {
		return "", err
	}

	task := database.NewTask("service scaling config",
		svc.ID, "", nil, 300)
	err = database.InsertTask(task)
	if err != nil {
		return "", err
	}
	go gd.serviceScale(svc, scale, task)

	return task.ID, nil
}

func (gd *Gardener) serviceScale(svc *Service, scale structs.PostServiceScaledRequest, task database.Task) (err error) {
	var storePendings []*pendingAllocStore
	svc.Lock()
	gd.scheduler.Lock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}

		if err == nil {
			task.Status = _StatusTaskDone
			err = svc.updateDescAfterScale(scale)
			if err != nil {
				logrus.Errorf("service %s update Description error:%s", svc.Name, err)
			}
		}

		if err != nil {
			err1 := gd.cancelStoreExtend(storePendings)
			if err1 != nil {
				err = fmt.Errorf("%s,%s", err, err1)
			}

			task.Status = _StatusTaskFailed
			task.Errors = err.Error()
		}

		err = database.UpdateTaskStatus(&task, task.Status, time.Now(), task.Errors)
		if err != nil {
			logrus.Errorf("task %s update error:%s", task.ID, err)
		}

		svc.Unlock()
		gd.scheduler.Unlock()
	}()

	pendings, err := svc.handleScaleUp(gd, scale.Type, scale.UpdateConfig)
	if err != nil {
		return err
	}

	storePendings, err = svc.volumesPendingExpension(gd, scale.Type, scale.Extensions)
	if err != nil {
		logrus.Error(err)

		return err
	}

	for i := range pendings {
		if pendings[i].svc == nil {
			pendings[i].svc = svc
		}
		err = pendings[i].containerUpdate()
		if err != nil {
			logrus.Error("container %s update error:%s", pendings[i].containerID, err)
			return err
		}
	}

	for _, pending := range storePendings {
		for _, lunID := range pending.sanStore {
			eng, err := pending.unit.getEngine()
			if err != nil {
				logrus.Errorf("%s %s", pending.unit.Name, err)
				return err
			}
			err = extendSanStoreageVG(eng.IP, lunID)
			if err != nil {
				logrus.Errorf("extend SanStoreageVG error:%s", err)
				return err
			}
		}
	}

	for _, pending := range storePendings {
		for _, lv := range pending.localStore {
			err = localVolumeExtend(pending.unit.engine.IP, lv)
			if err != nil {
				logrus.Errorf("unit %s update volume error %s", pending.unit.Name, err)
				return err
			}
			logrus.Debugf("unit %s update volume %v", pending.unit.Name, lv)
		}
	}

	return nil
}

func (p *pendingContainerUpdate) containerUpdate() error {
	if p.cpusetCpus != "" && p.config.CpusetCpus != "" {
		p.config.CpusetCpus = p.cpusetCpus
	}

	err := p.unit.updateContainer(p.config)
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

	return nil
}

func (svc *Service) getServiceDescription() (*structs.PostServiceRequest, error) {
	if svc.base != nil {
		return svc.base, nil
	}
	if svc.base == nil {
		if svc.Description == "" {
			table, err := database.GetService(svc.ID)
			if err != nil {
				return nil, err
			}
			svc.Service = table
		}
		if svc.Description != "" {
			err := json.NewDecoder(strings.NewReader(svc.Description)).Decode(svc.base)
			if err != nil {
				return nil, err
			}
		}
	}
	if svc.base == nil {
		return nil, fmt.Errorf("Service %s with null Description", svc.Name)
	}

	return svc.base, nil
}

func (svc *Service) updateDescAfterScale(scale structs.PostServiceScaledRequest) error {
	dsp, err := svc.getServiceDescription()
	if err != nil {
		return err
	}

	des := *dsp

	des.UpdateModuleConfig(scale.Type, *scale.UpdateConfig)

	des.UpdateModuleStore(scale.Type, scale.Extensions)

	buffer := bytes.NewBuffer(nil)
	err = json.NewEncoder(buffer).Encode(&des)
	if err != nil {
		return err
	}

	description := buffer.String()
	err = database.UpdateServcieDescription(svc.ID, description)
	if err != nil {
		return err
	}

	svc.Description = description
	svc.base = &des

	return nil
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
	configs := sys.GetConsulConfigs()
	if len(configs) == 0 {
		return fmt.Errorf("GetConsulConfigs error %v %v", err, sys)
	}

	entry.Debug("Service Delete... stop service & stop containers & rm containers & deregister")
	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	err = svc.Delete(gd, configs[0], horus, force, volumes, true, timeout)
	if err != nil {
		entry.Errorf("Service.Delete error:%s", err)

		return err
	}

	// delete database records relation svc.ID
	entry.Debug("DeteleServiceRelation...")
	err = database.DeteleServiceRelation(svc.ID, volumes)
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

func (svc *Service) Delete(gd *Gardener, config consulapi.Config, horus string, force, rmVolumes, recycle bool, timeout int) error {
	svc.Lock()
	defer svc.Unlock()

	for _, u := range svc.units {
		logrus.Debug(u.Name, " stop unit service")
		err := u.stopService()
		if err != nil {
			logrus.Errorf("container %s stop service error:%s", u.Name, err)

			err1 := checkContainerError(err)
			if err1 == errContainerNotFound || err1 == errContainerNotRunning {
				continue
			}
			if force && err.Error() == "EOF" {
				continue
			}

			return err
		}

		logrus.Debug(u.Name, " stop container")
		err = u.stopContainer(timeout)
		if err != nil {
			logrus.Errorf("container %s stop error:%s", u.Name, err)

			err1 := checkContainerError(err)
			if err1 == errContainerNotFound || err1 == errContainerNotRunning {
				continue
			}
			if force && err.Error() == "EOF" {
				continue
			}
			return err
		}
	}

	err := svc.removeContainers(force, rmVolumes)
	if err != nil {
		return err
	}

	volumes := make([]database.LocalVolume, 0, 10)

	for _, u := range svc.units {
		logrus.Debug(u.Name, " SelectVolumesByUnitID")
		lvs, err := database.SelectVolumesByUnitID(u.ID)
		if err != nil {
			logrus.Warnf("SelectVolumesByUnitID %s error:%s", u.Name, err)
			continue
		}
		volumes = append(volumes, lvs...)

		logrus.Debugf("recycle=%v", recycle)
		if recycle {
			logrus.Debug("DatacenterByEngine ", u.EngineID)
			dc, err := gd.DatacenterByEngine(u.EngineID)
			if err != nil || dc == nil || dc.storage == nil {
				continue
			}
			for i := range lvs {
				if !isSanVG(lvs[i].VGName) {
					logrus.Debug(i, lvs[i].VGName)
					continue
				}
				logrus.Debug("ListLUNByVgName ", lvs[i].VGName)
				list, err := database.ListLUNByVgName(lvs[i].VGName)
				if err != nil {
					logrus.Errorf("ListLUNByVgName %s error:%s", lvs[i].VGName, err)
					return err
				}
				for l := range list {
					logrus.Debug(i, "DelMapping & Recycle ", list[l].ID)
					err := dc.storage.DelMapping(list[l].ID)
					if err != nil {
						logrus.Errorf("DelMapping error:%s,unit:%s,lun:%s", err, u.Name, list[l].Name)
					}
					err = dc.storage.Recycle(list[l].ID, 0)
					if err != nil {
						logrus.Errorf("Recycle LUN error:%s,unit:%s,lun:%s", err, u.Name, list[l].Name)
					}
				}
			}
		}
	}

	// remove volumes
	for i := range volumes {
		logrus.Debug(i, len(volumes), "RemoveVolumes ", volumes[i].Name)
		found, err := gd.RemoveVolumes(volumes[i].Name)
		if !found {
			continue
		}
		if err != nil {
			logrus.Errorf("Remove Volumes %s error:%s", volumes[i].Name, err)
		}
	}

	logrus.Debug("deregisterInHorus")
	err = svc.deregisterInHorus(horus)
	if err != nil {
		logrus.Errorf("%s deregister In Horus error:%s", svc.Name, err)
	}

	logrus.Debug("deregisterServices")
	err = svc.deregisterServices(config)
	if err != nil {
		logrus.Errorf("%s deregister In consul error:%s", svc.Name, err)
	}

	return nil
}

var (
	errContainerNotFound   = errors.New("No Such Container")
	errContainerNotRunning = errors.New("Container Not Running")
)

func checkContainerError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "Error response from daemon: No such container") {
		return errContainerNotFound
	}

	if match, _ := regexp.MatchString(`Container \S* is not running`, err.Error()); match {
		return errContainerNotRunning
	}

	return err
}
