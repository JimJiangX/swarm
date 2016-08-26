package swarm

import (
	"bytes"
	"encoding/json"
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
	"github.com/pkg/errors"
	swm_structs "github.com/yiduoyunQ/sm/sm-svr/structs"
	"github.com/yiduoyunQ/smlib"
	"golang.org/x/net/context"
)

var (
	ErrServiceNotFound = errors.New("Service Not Found")
)

type Service struct {
	sync.RWMutex

	failureRetry int64

	database.Service
	base *structs.PostServiceRequest

	units  []*unit
	users  []database.User
	backup *database.BackupStrategy

	authConfig *types.AuthConfig
}

func NewService(svc database.Service, unitNum int) *Service {
	return &Service{
		Service: svc,
		units:   make([]*unit, 0, unitNum),
	}
}

func buildService(req structs.PostServiceRequest,
	authConfig *types.AuthConfig) (*Service, *database.Task, error) {
	// if warnings := ValidService(req); len(warnings) > 0 {
	//	 return nil, errors.New(strings.Join(warnings, ","))
	// }

	des, err := json.Marshal(req)
	if err != nil {
		logrus.Errorf("JSON Marshal Error:%s", err)
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
		Status:               statusServiceInit,
		BackupMaxSizeByte:    req.BackupMaxSize,
		BackupFilesRetention: req.BackupRetention,
		CreatedAt:            time.Now(),
	}

	strategy, err := newBackupStrategy(svc.ID, req.BackupStrategy)
	if err != nil {
		return nil, nil, err
	}

	_, nodeNum, err := parseServiceArch(req.Architecture)
	if err != nil {
		logrus.Error("Parse Service.Architecture", err)
		return nil, nil, err
	}
	sys, err := database.GetSystemConfig()
	if err != nil {
		return nil, nil, err
	}
	users := defaultServiceUsers(svc.ID, *sys)
	users = append(users, converteToUsers(svc.ID, req.Users)...)

	service := NewService(svc, nodeNum)

	service.Lock()
	defer service.Unlock()

	service.backup = strategy
	service.base = &req
	service.authConfig = authConfig
	service.users = users
	atomic.StoreInt64(&svc.Status, statusServcieBuilding)

	task := database.NewTask(service.Name, _Service_Create_Task, service.ID, "create service", nil, 0)

	err = database.TxSaveService(service.Service, service.backup, &task, service.users)

	return service, &task, err
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

	users := converteToUsers(svc.ID, req)
	update := make([]database.User, 0, len(req))
	addition := make([]database.User, 0, len(req))
	for i := range users {
		exist := false
		for u := range svc.users {
			if svc.users[u].Username == users[i].Username {
				users[i].ID = svc.users[u].ID
				users[i].CreatedAt = svc.users[u].CreatedAt

				update = append(update, users[i])
				exist = true
				break
			}
		}
		if !exist {
			addition = append(addition, users[i])
		}
	}

	addr, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		return 0, err
	}

	swmUsers := converteToSWM_Users(addition)
	for i := range swmUsers {

		code = 201
		err := smlib.AddUser(addr, port, swmUsers[i])
		if err != nil {
			logrus.Errorf("%s add user error:%s", addr, err)
			return 0, err
		}
		logrus.Debug("Add User:", swmUsers[i].UserName)
	}

	swmUsers = converteToSWM_Users(update)
	for i := range swmUsers {
		err := smlib.UptUser(addr, port, swmUsers[i])
		if err != nil {
			logrus.Errorf("%s update user error:%s", addr, err)
			return 0, err
		}
		logrus.Debug("Update User:", swmUsers[i].UserName)
	}

	err = database.TxUpdateUsers(addition, update)
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

	users := converteToSWM_Users(list)
	for i := range users {
		err := smlib.DelUser(addr, port, users[i])
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
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      _User_Type_DB,
			Username:  sys.MonitorUsername,
			Password:  sys.MonitorPassword,
			Role:      _User_Monitor,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      _User_Type_DB,
			Username:  sys.ApplicationUsername,
			Password:  sys.ApplicationPassword,
			Role:      _User_Application,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      _User_Type_DB,
			Username:  sys.DBAUsername,
			Password:  sys.DBAPassword,
			Role:      _User_DBA,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      _User_Type_DB,
			Username:  sys.DBUsername,
			Password:  sys.DBPassword,
			Role:      _User_DB,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      _User_Type_DB,
			Username:  sys.ReplicationUsername,
			Password:  sys.ReplicationPassword,
			Role:      _User_Replication,
			ReadOnly:  false,
			CreatedAt: now,
		},
	}
}

func converteToUsers(service string, users []structs.User) []database.User {
	out := make([]database.User, 0, len(users))
	now := time.Now()

	for i := range users {

		switch {
		case users[i].Type == _User_Type_DB:
		case users[i].Type == _User_Type_Proxy:

		case strings.ToLower(users[i].Type) == strings.ToLower(_User_Type_DB):

			users[i].Type = _User_Type_DB

		case strings.ToLower(users[i].Type) == strings.ToLower(_User_Type_Proxy):

			users[i].Type = _User_Type_Proxy

		default:
			logrus.WithField("Service", service).Warnf("skip:%s Role='%s'", users[i].Username, users[i].Type)
			continue
		}

		switch {
		case users[i].Role == _User_DB:
		case users[i].Role == _User_Application:
		case users[i].Role == _User_Check:
		case users[i].Role == _User_DBA:
		case users[i].Role == _User_Monitor:
		case users[i].Role == _User_Replication:

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_DB):

			users[i].Role = _User_DB

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_Application):

			users[i].Role = _User_Application

		default:
			logrus.WithField("Service", service).Warnf("skip:%s Role='%s'", users[i].Username, users[i].Role)
			continue
		}

		out = append(out, database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      users[i].Type,
			Username:  users[i].Username,
			Password:  users[i].Password,
			Role:      users[i].Role,
			ReadOnly:  users[i].ReadOnly,
			Blacklist: users[i].Blacklist,
			Whitelist: users[i].Whitelist,
			CreatedAt: now,
		})
	}

	return out
}

func converteToSWM_Users(users []database.User) []swm_structs.User {
	out := make([]swm_structs.User, 0, len(users))

	for i := range users {
		switch {
		case users[i].Type == _User_Type_Proxy:
		case users[i].Type == _User_Type_DB:

		case strings.ToLower(users[i].Type) == strings.ToLower(_User_Type_DB):

			users[i].Type = _User_Type_DB

		case strings.ToLower(users[i].Type) == strings.ToLower(_User_Type_Proxy):

			users[i].Type = _User_Type_Proxy

		default:
			logrus.WithField("Service", users[i].ServiceID).Warnf("skip:%s Type='%s'", users[i].Username, users[i].Type)
			continue
		}

		switch {
		case users[i].Role == _User_DB:
		case users[i].Role == _User_Application:

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_DB):

			users[i].Role = _User_DB

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_Application):

			users[i].Role = _User_Application

		default:
			logrus.WithField("Service", users[i].ServiceID).Warnf("skip:%s Role='%s'", users[i].Username, users[i].Role)
			continue
		}

		out = append(out, swm_structs.User{
			Id:        users[i].ID,
			Type:      users[i].Type,
			UserName:  users[i].Username,
			Password:  users[i].Password,
			Role:      users[i].Role,
			BlackList: users[i].Blacklist,
			WhiteList: users[i].Whitelist,
			ReadOnly:  users[i].ReadOnly,
		})
	}

	return out
}

func (svc *Service) getUnit(nameOrID string) (*unit, error) {
	for _, u := range svc.units {
		if u.ID == nameOrID || u.Name == nameOrID {
			return u, nil
		}
	}

	return nil, fmt.Errorf("Unit Not Found,%s", nameOrID)
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

func (svc *Service) SaveToDB(task *database.Task) error {
	return database.TxSaveService(svc.Service, svc.backup, task, svc.users)
}

func (gd *Gardener) GetService(nameOrID string) (*Service, error) {
	gd.RLock()

	for i := range gd.services {
		if gd.services[i].ID == nameOrID || gd.services[i].Name == nameOrID {
			gd.RUnlock()

			return gd.services[i], nil
		}
	}

	gd.RUnlock()

	return gd.rebuildService(nameOrID)
}

func (gd *Gardener) rebuildService(nameOrID string) (*Service, error) {
	service, err := database.GetService(nameOrID)
	if err != nil {
		return nil, ErrServiceNotFound
	}

	base := &structs.PostServiceRequest{}
	if len(service.Description) > 0 {
		err := json.Unmarshal([]byte(service.Description), base)
		if err != nil {
			logrus.WithError(err).Warn("JSON Unmarshal Service.Description")
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
		logrus.WithError(err).Error("Registry Auth Config")
		return nil, err
	}
	users, err := database.ListUsersByService(service.ID, "")
	if err != nil {
		logrus.Errorf("List Users By Service %s,error:%s", service.ID, err)
		logrus.WithField("Service", service.ID).Errorf("List Users By Service Error:%s", err)
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
	svc.users = users
	svc.authConfig = authConfig

	gd.Lock()

	exist := false
	for i := range gd.services {
		if gd.services[i].ID == svc.ID {
			gd.services[i] = svc
			exist = true
			break
		}
	}
	if !exist {
		gd.services = append(gd.services, svc)
	}
	gd.Unlock()

	logrus.WithField("NameOrID", nameOrID).Debug("rebuild Service")

	return svc, nil
}

func (gd *Gardener) CreateService(req structs.PostServiceRequest) (*Service, string, string, error) {
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		logrus.Error("get Registry Auth Config", err)
		return nil, "", "", err
	}

	svc, task, err := buildService(req, authConfig)
	if err != nil {
		logrus.Error("Build Service Error:", err)

		return svc, "", task.ID, err
	}

	strategyID := ""
	if svc.backup != nil {
		strategyID = svc.backup.ID
	}

	svc.failureRetry = gd.createRetry

	logrus.WithFields(logrus.Fields{
		"ServcieName": svc.Name,
		"ServiceID":   svc.ID,
	}).Info("Service Saved Into Database")

	err = gd.AddService(svc)
	if err != nil {
		logrus.WithField("ServiceName", svc.Name).Errorf("Service Add to Gardener Error:%s", err)

		return svc, strategyID, task.ID, err
	}

	background := func(context.Context) error {
		svc.RLock()
		defer svc.RUnlock()

		err := gd.serviceScheduler(svc, task)
		if err != nil {
			logrus.Error("Service Add To Scheduler", err)

			return err
		}
		logrus.Debugf("Service %s Scheduler OK!", svc.Name)

		err = gd.serviceExecute(svc)
		if err != nil {
			logrus.Errorf("Service %s,execute error:%s", svc.Name, err)

			return err
		}

		if svc.backup != nil {
			bs := NewBackupJob(svc)
			err = gd.RegisterBackupStrategy(bs)
			if err != nil {
				logrus.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err)
			}
		}

		return nil
	}

	updater := func(code int, msg string) error {
		svcStatus := atomic.LoadInt64(&svc.Status)

		return database.TxSetServiceStatus(&svc.Service, task, svcStatus, int64(code), time.Now(), msg)
	}

	worker := NewAsyncTask(context.Background(), background, nil, updater, 10*time.Minute)
	err = worker.Run()

	return svc, strategyID, task.ID, err
}

func (svc *Service) StartService() (err error) {
	err = svc.statusCAS(statusServiceNoContent, statusServiceStarting)
	if err != nil {
		logrus.Warning(err)

		err = svc.statusCAS(statusServiceStartFailed, statusServiceStarting)
		if err != nil {
			logrus.Error(err)

			return err
		}
	}

	svc.Lock()
	defer func() {
		if err != nil {
			svc.SetServiceStatus(statusServiceStartFailed, time.Now())
		} else {
			svc.SetServiceStatus(statusServiceNoContent, time.Now())
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

	return nil
}

func (svc *Service) startContainers() error {
	for _, u := range svc.units {
		code, msg := int64(statusUnitStarting), ""

		err := u.startContainer()
		if err != nil {
			code, msg = statusUnitStartFailed, err.Error()

			logrus.Errorf("%s start container error:%s", u.Name, err)
		}

		_err := database.TxUpdateUnitStatus(&u.Unit, code, msg)
		if err != nil {
			logrus.WithField("Unit", u.Name).Errorf("Update Unit Status,status=%d,LatestError=%s,%v", code, msg, _err)

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
			logrus.Errorf("Unit %s:%s,defaultUserConfig error,%s", u.Name, u.ImageName, err)

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
	var (
		swm   *unit
		funcs = make([]func() error, len(svc.units))
	)
	for i := range svc.units {
		if svc.units[i].Type == _SwitchManagerType {
			swm = svc.units[i]
			continue
		}
		funcs[i] = svc.units[i].initService
	}

	if swm != nil {
		err := swm.initService()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Service": svc.Name,
				"Unit":    swm.Name,
			}).WithError(err).Error("Init service")

			return err
		}
	}

	err := GoConcurrency(funcs)
	if err != nil {
		logrus.WithField("Service", svc.Name).WithError(err).Error("Init services")
	}

	return err
}

func (svc *Service) checkStatus(expected int64) error {
	val := atomic.LoadInt64(&svc.Status)
	if val == expected {
		return nil
	}

	return fmt.Errorf("Service %s,Status Conflict:expected %d but got %d", svc.Name, expected, val)
}

func (svc *Service) statusCAS(expected, value int64) error {
	if atomic.CompareAndSwapInt64(&svc.Status, expected, value) {
		return nil
	}

	return fmt.Errorf("Service %s,Status Conflict:expected %d but got %d", svc.Name, expected, atomic.LoadInt64(&svc.Status))
}

func (svc *Service) startService() error {
	var swm *unit
	funcs := make([]func() error, len(svc.units))
	for i := range svc.units {
		if svc.units[i].Type == _SwitchManagerType {
			swm = svc.units[i]
			continue
		}
		funcs[i] = svc.units[i].startService
	}

	err := GoConcurrency(funcs)
	if err != nil {
		logrus.WithField("Service", svc.Name).WithError(err).Error("start services")
	}

	if err == nil && swm != nil {

		err = swm.startService()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Service": svc.Name,
				"Unit":    swm.Name,
			}).WithError(err).Error("Service start service")
		}
	}

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

func (svc *Service) StopService() (err error) {
	err = svc.statusCAS(statusServiceNoContent, statusServiceStoping)
	if err != nil {
		logrus.Warning(err)

		err = svc.statusCAS(statusServiceStopFailed, statusServiceStoping)
		if err != nil {
			logrus.Error(err)

			return err
		}
	}

	svc.Lock()
	defer func() {
		if err != nil {
			svc.SetServiceStatus(statusServiceStopFailed, time.Now())
		} else {
			svc.SetServiceStatus(statusServiceNoContent, time.Now())
		}
		svc.Unlock()
	}()

	swm, err := svc.getSwithManagerUnit()
	if err == nil && swm != nil {
		err = swm.stopService()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Service": svc.Name,
				"Unit":    swm.Name,
			}).WithError(err).Error("stop service")

			err = checkContainerError(err)
			if err != errContainerNotRunning && err != errContainerNotFound {
				return err
			}
		}
	}

	units, err := svc.getUnitByType(_UpsqlType)
	if err != nil {
		logrus.WithField("Service", svc.Name).WithError(err).Error("get unit by type")

		return err
	}

	funcs := make([]func() error, len(units))
	for i := range units {

		funcs[i] = units[i].stopService
	}

	err = GoConcurrency(funcs)
	if err != nil {
		logrus.WithField("Service", svc.Name).WithError(err).Error("stop services")

		if _err, ok := err.(_errors); ok {
			errs := _err.Split()
			pass := true
			for i := range errs {
				err := checkContainerError(errs[i])
				if err != errContainerNotRunning && err != errContainerNotFound {
					pass = false
					break
				}
			}
			if pass {
				return nil
			}
		}
	}

	return err
}

func (svc *Service) stopService() error {
	var swm *unit
	funcs := make([]func() error, len(svc.units))

	for i := range svc.units {
		if svc.units[i].Type == _SwitchManagerType {
			swm = svc.units[i]
			continue
		}

		funcs[i] = svc.units[i].stopService
	}

	if swm != nil {
		err := swm.stopService()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Service": svc.Name,
				"Unit":    swm.Name,
			}).WithError(err).Error("stop service")

			err = checkContainerError(err)
			if err != errContainerNotRunning && err != errContainerNotFound {
				return err
			}
		}
	}

	err := GoConcurrency(funcs)
	if err != nil {
		logrus.Errorf("Service %s stop service error:%s", svc.Name, err)
		if _err, ok := err.(_errors); ok {
			errs := _err.Split()
			pass := true
			for i := range errs {
				err := checkContainerError(errs[i])
				if err != errContainerNotRunning && err != errContainerNotFound {
					pass = false
					break
				}
			}
			if pass {
				return nil
			}
		}
	}

	return err
}

func (svc *Service) RemoveContainers(force, rmVolumes bool) error {
	if !force {
		err := svc.checkStatus(statusServiceNoContent)
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

		atomic.StoreInt64(&u.Status, statusUnitDeleting)

		logrus.Debug(u.Name, " remove Container")
		err := u.removeContainer(force, rmVolumes)
		if err != nil {
			logrus.Errorf("container %s remove,-f=%v -v=%v,error:%s", u.Name, force, rmVolumes, err)
			if err == errEngineIsNil {
				continue
			}
			if err := checkContainerError(err); err == errContainerNotFound {
				continue
			}

			return err
		}
		logrus.Debugf("container %s removed", u.Name)
	}

	return nil
}

func (svc *Service) ModifyUnitConfig(_type string, config map[string]interface{}) (err error) {
	if _type == _ProxyType || _type == _SwitchManagerType {
		return svc.UpdateUnitConfig(_type, config)
	}

	if _type != _UpsqlType {
		return errors.Errorf("Unsupported Type:'%s'", _type)
	}

	svc.Lock()
	defer svc.Unlock()

	units, err := svc.getUnitByType(_type)
	if err != nil {
		return err
	}

	dba, found := database.User{}, false
	for i := range svc.users {
		if svc.users[i].Username == _User_DBA {
			dba = svc.users[i]
			found = true
			break
		}
	}
	if !found {
		return errors.Errorf("Service %s missing User:%s", svc.Name, _User_DBA)
	}

	for key, val := range config {
		delete(config, key)

		if parts := strings.SplitN(key, "::", 2); len(parts) == 1 {
			key = "default::" + key
		}

		config[strings.ToLower(key)] = val
	}

	u := units[0]

	if u.parent == nil || u.configParser == nil {
		if u.ConfigID == "" {
			return errors.Errorf("unit %s infomation bug", u.Name)
		}

		data, err := database.GetUnitConfigByID(u.ConfigID)
		if err == nil {
			u.parent = data
		} else {
			return err
		}

		if err = u.factory(); err != nil {
			return err
		}

		out, ok := u.CanModify(config)
		if !ok {
			return errors.Errorf("Cannot Modify UnitConfig,Key:%s", out)
		}
	}

	template := [...]string{"mysql", "-u%s", "-p%s", "-S", "/DBAASDAT/upsql.sock", "-e", "SET GLOBAL %s = %v;"}
	template[1] = fmt.Sprintf(template[1], dba.Username)
	template[2] = fmt.Sprintf(template[2], dba.Password)
	const length, last = len(template), len(template) - 1

	copyContent := u.parent.Content
	cmdRollback := make([]*unit, 0, len(units))
	cnfRollback := make([]*unit, 0, len(units))
	cmdList := make([][length]string, 0, len(config))
	originalCmds := make([][length]string, 0, len(config))

	defer func() {
		if err == nil {
			return
		}

		for _, u := range cmdRollback {
			engine, _err := u.Engine()
			if _err != nil {
				logrus.Error(u.Name, _err)
				continue
			}

			for i := range originalCmds {

				inspect, _err := containerExec(context.Background(), engine, u.ContainerID, originalCmds[i][:], false)
				if inspect.ExitCode != 0 {
					_err = errors.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", u.Name, originalCmds[i], inspect.ExitCode, inspect, err)
				}
				if _err != nil {
					logrus.Errorf("Rollback:%s", _err)
				}
			}

			for _, u := range cnfRollback {
				u.parent.Content = copyContent

				_err := u.CopyConfig(nil)
				if _err != nil {
					logrus.Errorf("ConfigFile Rollback:%s", err)
				}
			}
		}
	}()

	configer, err := u.ParseData([]byte(copyContent))
	if err != nil {
		return err
	}

	for key, val := range config {
		oldValue := configer.String(key)
		cmds := template
		old := template
		parts := strings.SplitAfterN(key, "::", 2)
		if len(parts) == 2 {
			key = parts[1]
		} else if len(parts) == 1 {
			key = parts[0]
		}

		err = configer.Set(key, fmt.Sprintf("%v", val))
		if err != nil {
			return err
		}

		key = strings.Replace(key, "-", "_", -1)

		cmds[last] = fmt.Sprintf(cmds[last], key, val)
		cmdList = append(cmdList, cmds)

		old[last] = fmt.Sprintf(old[last], key, oldValue)
		originalCmds = append(originalCmds, old)
	}

	for _, u := range units {
		engine, err := u.Engine()
		if err != nil {
			return err
		}

		cmdRollback = append(cmdRollback, u)

		for i := range cmdList {

			inspect, err := containerExec(context.Background(), engine, u.ContainerID, cmdList[i][:], false)
			if inspect.ExitCode != 0 {
				err = errors.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmdList[i], inspect.ExitCode, inspect, err)
			}
			if err != nil {
				logrus.Error(err)
				return err
			}
		}
	}

	for _, u := range units {
		cnfRollback = append(cnfRollback, u)

		err = u.CopyConfig(config)
		if err != nil {
			return err
		}
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

	units, err := svc.getUnitByType(_type)
	if err != nil {
		return err
	}

	for _, u := range units {
		keys, ok := u.CanModify(config)
		if !ok {
			return errors.Errorf("Illegal keys:%s,Or keys unable to modified", keys)
		}

		err := u.CopyConfig(config)
		if err != nil {
			return err
		}

		if u.MustRestart(config) {

			// disable restart Service for now
			logrus.WithField("Container", u.Name).Warn("Should restart service to make new config file works")
			return nil

			err := u.stopService()
			if err != nil {
				logrus.Errorf("%s stop Service error,%s", u.Name, err)
			}

			err = u.startService()
			if err != nil {
				logrus.Errorf("%s start Service error,%s", u.Name, err)
				return err
			}
		}
	}

	return nil
}

func (svc *Service) initTopology() error {
	swm, _ := svc.getSwithManagerUnit()
	sqls, _ := svc.getUnitByType(_UpsqlType)
	proxys, _ := svc.getUnitByType(_ProxyType)

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
		proxyNames[i] = proxys[i].Name
	}

	var (
		dba, replicater database.User
		users           = converteToSWM_Users(svc.users)
	)
	for i := range svc.users {
		if svc.users[i].Role == _User_DBA {
			dba = svc.users[i]
		} else if svc.users[i].Role == _User_Replication {
			replicater = svc.users[i]
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
		return errors.Errorf("get %d units by type:'%s'", num, _UpsqlType)
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

	topolony := swm_structs.MgmPost{
		DbaasType:           arch,                //  string   `json:"dbaas-type"`
		DbRootUser:          dba.Username,        //  string   `json:"db-root-user"`
		DbRootPassword:      dba.Password,        //  string   `json:"db-root-password"`
		DbReplicateUser:     replicater.Username, //  string   `json:"db-replicate-user"`
		DbReplicatePassword: replicater.Password, //  string   `json:"db-replicate-password"`
		SwarmApiVersion:     "1.22",              //  string   `json:"swarm-api-version,omitempty"`
		ProxyNames:          proxyNames,          //  []string `json:"proxy-names"`
		Users:               users,               //  []User   `json:"users"`
		DataNode:            dataNodes,           //  map[string]DatabaseInfo `json:"data-node"`
	}

	err = smlib.InitSm(addr, port, topolony)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			time.Sleep(time.Second * 3)
			err = smlib.InitSm(addr, port, topolony)
		}
	}

	return errors.Wrap(err, "init topology")
}

func (svc *Service) registerServices() (err error) {
	for _, u := range svc.units {
		err = registerHealthCheck(u, svc)
		if err != nil {

			return err
		}
	}

	return nil
}

func (svc *Service) deregisterServices() error {
	for _, u := range svc.units {
		eng, err := u.Engine()
		if err != nil {
			logrus.Error(err)
			continue
		}
		err = deregisterHealthCheck(eng.IP, u.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) registerToHorus(user, password string, agentPort int) error {
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

	return registerToHorus(params...)
}

func (svc *Service) deregisterInHorus() error {
	endpoints := make([]string, len(svc.units))

	for i, u := range svc.units {
		endpoints[i] = u.ID
	}

	err := deregisterToHorus(false, endpoints...)
	if err != nil {
		logrus.WithField("Endpoints", endpoints).Errorf("Deregister To Horus")

		err = deregisterToHorus(true, endpoints...)
		if err != nil {
			logrus.WithField("Endpoints", endpoints).Errorf("Deregister To Horus,force=true")

			return err
		}
	}

	return nil
}

func (svc *Service) getUnitByType(_type string) ([]*unit, error) {
	units := make([]*unit, 0, len(svc.units))
	for _, u := range svc.units {
		if u.Type == _type {
			units = append(units, u)
		}
	}
	if len(units) > 0 {
		return units, nil
	}

	return nil, errors.Errorf("Service:%s,not found unit by type '%s'", svc.Name, _type)
}

func (svc *Service) getSwithManagerUnit() (*unit, error) {
	units, err := svc.getUnitByType(_UnitRole_SwitchManager)
	if err != nil {
		return nil, err
	}

	return units[0], nil
}

func (svc *Service) getSwitchManagerAddr() (string, int, error) {
	swm, err := svc.getSwithManagerUnit()
	if err != nil {
		return "", 0, err
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

func (svc *Service) GetSwitchManagerAddr() (string, error) {
	svc.RLock()

	host, port, err := svc.getSwitchManagerAddr()

	svc.RUnlock()

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%d", host, port), nil
}

func (svc *Service) getSwitchManagerAndMaster() (string, int, *unit, error) {
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

	masterName := ""
loop:
	for _, val := range topology.DataNodeGroup {
		for id, node := range val {
			if strings.EqualFold(node.Type, _UnitRole_Master) {
				masterName = id

				break loop
			}
		}
	}

	if masterName == "" {
		// Not Found master DB
		return addr, port, nil, fmt.Errorf("Master Unit Not Found")
	}

	master, err := svc.getUnit(masterName)

	return addr, port, master, err
}

func (gd *Gardener) UnitIsolate(nameOrID string) error {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		logrus.Errorf("Get Unit error:%s %s", err, nameOrID)
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

func (svc *Service) isolate(unitName string) error {
	u, err := svc.getUnit(unitName)
	if err != nil {

		logrus.Warning(err)

	} else if u.Type == _SwitchManagerType {

		return fmt.Errorf("Cann't Isolate Unit '%s:%s'", u.Type, unitName)
	}

	ip, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		logrus.Errorf("get SwitchManager Addr error:%s", err)
		return err
	}

	logrus.Debugf("Service %s Isolate Unit %s,%s:%d", svc.Name, unitName, ip, port)

	err = smlib.Isolate(ip, port, unitName)

	if err != nil {
		logrus.Errorf("Isolate %s error:%s:%d %s", unitName, ip, port, err)
	}

	return err
}

func (gd *Gardener) UnitSwitchBack(nameOrID string) error {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		logrus.Errorf("Get Unit error:%s %s", err, nameOrID)
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

func (svc *Service) switchBack(unitName string) error {
	u, err := svc.getUnit(unitName)
	if err != nil {

		logrus.Warning(err)

	} else if u.Type == _SwitchManagerType {

		return fmt.Errorf("Cann't SwitchBack Unit '%s:%s'", u.Type, unitName)
	}

	ip, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		logrus.Errorf("get SwitchManager Addr error:%s", err)
		return err
	}

	logrus.Debugf("Service %s Switchback Unit %s,%s:%d", svc.Name, unitName, ip, port)

	err = smlib.Recover(ip, port, unitName)
	if err != nil {
		logrus.Errorf("switchBack %s error:%s:%d %s", unitName, ip, port, err)
	}

	return err
}

func (gd *Gardener) TemporaryServiceBackupTask(service, nameOrID string) (string, error) {
	if nameOrID != "" {
		u, err := database.GetUnit(nameOrID)
		if err != nil {
			logrus.Errorf("Not Found Unit '%s',Error:%s", nameOrID, err)
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

	ok, err := checkBackupFiles(svc.ID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.Errorf("Service %s,No More Space For Backup Task", svc.Name)

	}

	var backup *unit
	if nameOrID != "" {
		backup, err = svc.getUnit(nameOrID)
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
		Name:      backup.Name + "_backup_manually_" + utils.TimeToString(now),
		Type:      "full",
		ServiceID: svc.ID,
		Spec:      "manually",
		Valid:     now,
		Enabled:   false,
		BackupDir: sys.BackupDir,
		Timeout:   2 * 60 * 60,
		CreatedAt: now,
	}

	task := database.NewTask(backup.Name, _Backup_Manual_Task, backup.ID, "", nil, strategy.Timeout)
	task.Status = statusTaskCreate
	err = database.TxInsertBackupStrategyAndTask(strategy, task)
	if err != nil {
		logrus.Errorf("TxInsert BackupStrategy And Task Erorr:%s", err)
		return "", err
	}

	go backupTask(master, &task, strategy, func() error {
		var err error
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
		_err := smlib.UnLock(addr, port)
		if _err != nil {
			logrus.Errorf("switch_manager %s:%d Unlock Error:%s", addr, port, _err)
			err = errors.Errorf("%s,%s", err, _err)
		}

		return err
	})

	return task.ID, nil
}

type pendingContainerUpdate struct {
	containerID string
	cpusetCpus  string
	unit        *unit
	svc         *Service
	engine      *cluster.Engine
	config      container.UpdateConfig
}

func (gd *Gardener) ServiceScale(name string, scale structs.PostServiceScaledRequest) error {
	svc, err := gd.GetService(name)
	if err != nil {
		return err
	}

	err = ValidateServiceScale(svc, scale)
	if err != nil {
		return err
	}

	err = gd.serviceScale(svc, scale)
	if err != nil {
		logrus.Errorf("Service  %s:%s Scale Error:%s", svc.Name, scale.Type, err)
	}

	return err
}

func (gd *Gardener) serviceScale(svc *Service, scale structs.PostServiceScaledRequest) (err error) {
	var storePendings []*pendingAllocStore
	svc.Lock()
	gd.scheduler.Lock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}

		if err == nil {
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
			logrus.Errorf("container %s update error:%s", pendings[i].containerID, err)
			return err
		}
	}

	for _, pending := range storePendings {
		for i := range pending.sanStore {
			eng, err := pending.unit.Engine()
			if err != nil {
				logrus.Errorf("%s %s", pending.unit.Name, err)
				return err
			}
			err = extendSanStoreageVG(eng.IP, pending.sanStore[i])
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

func (gd *Gardener) RemoveService(nameOrID string, force, volumes bool, timeout int) error {
	entry := logrus.WithFields(logrus.Fields{
		"Name":    nameOrID,
		"force":   force,
		"volumes": volumes,
	})
	entry.Info("Removing Service...")

	gd.Lock()
	for i := range gd.services {
		if gd.services[i].ID == nameOrID || gd.services[i].Name == nameOrID {
			gd.services = append(gd.services[:i], gd.services[i+1:]...)
			break
		}
	}
	gd.Unlock()

	entry.Debug("GetService From Gardener...")
	svc, err := gd.GetService(nameOrID)
	if err != nil {
		if err == ErrServiceNotFound {
			return nil
		}
		entry.Errorf("GetService From Gardener error:%s", err)
	}

	entry.Debug("Service Delete... stop service & stop containers & rm containers & deregister")

	err = svc.Delete(gd, force, volumes, true, timeout)
	if err != nil {
		entry.Errorf("Service.Delete error:%s", err)

		return err
	}

	// delete database records relation svc.ID
	entry.Debug("DeteleServiceRelation...")
	err = database.DeteleServiceRelation(svc.ID, volumes)
	if err != nil {
		entry.Errorf("DeteleServiceRelation error:%s", err)
	}

	entry.Debug("Remove Service From Gardener...")
	gd.Lock()
	for i := range gd.services {
		if gd.services[i].ID == nameOrID || gd.services[i].Name == nameOrID {
			gd.services = append(gd.services[:i], gd.services[i+1:]...)
			break
		}
	}
	gd.Unlock()

	if svc.backup != nil {
		err = gd.RemoveCronJob(svc.backup.ID)
		if err != nil {
			entry.Errorf("RemoveCronJob %s error:%s", svc.backup.ID, err)

			return err
		}
	}

	return nil
}

func (svc *Service) Delete(gd *Gardener, force, rmVolumes, recycle bool, timeout int) error {
	svc.Lock()
	defer svc.Unlock()

	funcs := make([]func() error, len(svc.units))
	for i := range svc.units {
		u := svc.units[i]
		funcs[i] = func() error {
			if _, err := u.Engine(); err == errEngineIsNil {
				logrus.Warnf("Remove Unit %s,error:%s", u.Name, err)
				return nil
			}

			logrus.Debug(u.Name, " stop unit service")
			err := u.forceStopService()
			if err != nil &&
				u.container.Info.State.Running &&
				err.Error() != "EOF" {

				return errors.Wrapf(err, "%s forceStopService error", u.Name)
			}

			logrus.Debug(u.Name, " stop container")
			err = u.forceStopContainer(timeout)
			if err != nil {
				if !u.container.Info.State.Running {
					return nil
				}

				if err.Error() == "EOF" {
					return nil
				}
				err = errors.Wrapf(err, "%s forceStopContainer error", u.Name)
			}

			return err
		}
	}

	err := GoConcurrency(funcs)
	if err != nil {
		logrus.Errorf("Service %s stop error:%s", svc.Name, err)

		pass := true
		_err, ok := err.(_errors)
		if ok {
			errs := _err.Split()
			for i := range errs {
				err := checkContainerError(errs[i])
				if err != errContainerNotRunning && err != errContainerNotFound {
					pass = false
					break
				}
			}
		}
		if !pass || !ok {
			return err
		}
	}

	err = svc.removeContainers(force, rmVolumes)
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
		found, err := gd.RemoveVolumes(volumes[i].Name)
		if err != nil {
			logrus.Errorf("Remove Volumes %s,Found=%t,%s", volumes[i].Name, found, err)
			continue
		}

		logrus.Debug(i, len(volumes), "RemoveVolume ", volumes[i].Name)
	}

	logrus.Debug("deregisterInHorus")
	err = svc.deregisterInHorus()
	if err != nil {
		logrus.Errorf("%s deregister In Horus error:%s", svc.Name, err)
	}

	logrus.Debug("deregisterServices")

	err = svc.deregisterServices()
	if err != nil {
		logrus.Errorf("%s deregister In consul error:%s", svc.Name, err)
	}

	err = deleteConsulKVTree(svc.ID)
	if err != nil {
		logrus.Errorf("Delete Consul KV:%s,%s", svc.ID, err)
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

	if err == errContainerNotRunning || err == errContainerNotFound {
		return err
	}

	if strings.Contains(err.Error(), "Error response from daemon: No such container") {
		return errContainerNotFound
	}

	if match, _ := regexp.MatchString(`Container \S* is not running`, err.Error()); match {
		return errContainerNotRunning
	}

	return err
}
