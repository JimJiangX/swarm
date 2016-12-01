package swarm

import (
	"bytes"
	"database/sql"
	"encoding/json"
	stderrors "errors"
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
	errServiceNotFound           = stderrors.New("service not found")
	ErrSwitchManagerUnitNotFound = stderrors.New("unit not found,type:" + _SwitchManagerType)
)

// Service a set of units
type Service struct {
	failureRetry int

	sync.RWMutex

	statusLock statusLock

	database.Service

	base *structs.PostServiceRequest

	units  []*unit
	users  []database.User
	backup *database.BackupStrategy

	authConfig *types.AuthConfig
}

func newService(service database.Service, unitNum, retries int) *Service {
	return &Service{
		failureRetry: retries,
		Service:      service,
		units:        make([]*unit, 0, unitNum),
		statusLock:   defaultServiceStatusLock(service.ID),
	}
}

func buildService(req structs.PostServiceRequest, authConfig *types.AuthConfig, sysConfig database.Configurations, retries int) (*Service, *database.Task, error) {
	if req.ID == "" {
		req.ID = utils.Generate64UUID()
	}

	desc, err := json.Marshal(req)
	if err != nil {
		logrus.WithError(err).Error("JSON marshal error")
	}

	service := database.Service{
		ID:           req.ID,
		Name:         req.Name,
		Desc:         string(desc),
		Architecture: req.Architecture,
		BusinessCode: req.BusinessCode,
		AutoHealing:  req.AutoHealing,
		AutoScaling:  req.AutoScaling,
		// HighAvailable:        req.HighAvailable,
		Status:               statusServcieBuilding,
		BackupMaxSizeByte:    req.BackupMaxSize,
		BackupFilesRetention: req.BackupRetention,
		CreatedAt:            time.Now(),
	}

	strategy, err := newBackupStrategy(service.ID, req.BackupStrategy)
	if err != nil {
		return nil, nil, err
	}

	_, nodeNum, err := parseServiceArch(req.Architecture)
	if err != nil {
		return nil, nil, err
	}

	users := defaultServiceUsers(service.ID, sysConfig)
	users = append(users, converteToUsers(service.ID, req.Users)...)

	svc := newService(service, nodeNum, retries)

	svc.Lock()
	defer svc.Unlock()

	svc.backup = strategy
	svc.base = &req
	svc.authConfig = authConfig
	svc.users = users

	task := database.NewTask(svc.Name, serviceCreateTask, svc.ID, "create service", nil, 0)

	err = database.TxSaveService(svc.Service, svc.backup, &task, svc.users)

	return svc, &task, err
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
			return nil, errors.Wrap(err, "parse BackupStrategy.Valid to time.Time")
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

func (svc *Service) replaceBackupStrategy(req structs.BackupStrategy) (*database.BackupStrategy, error) {
	backup, err := newBackupStrategy(svc.ID, &req)
	if err != nil || backup == nil {
		return nil, errors.Errorf("with non Backup Strategy,%+v", err)
	}

	err = database.InsertBackupStrategy(*backup)
	if err != nil {
		return nil, err
	}

	svc.Lock()
	svc.backup = backup
	svc.Unlock()

	return backup, nil
}

// DeleteServiceBackupStrategy delete the strategy
func DeleteServiceBackupStrategy(strategy string) error {
	backup, err := database.GetBackupStrategy(strategy)
	if err != nil {
		return err
	}

	if backup.Enabled {
		return errors.Errorf("BackupStrategy %s is using,Cannot delete", strategy)
	}

	err = database.DeleteBackupStrategy(strategy)

	return err
}

// AddServiceUsers add users into service
func (svc *Service) AddServiceUsers(req []structs.User) (int, error) {
	done, val, err := svc.statusLock.CAS(statusServiceUsersUpdating, isStatusNotInProgress)
	if err != nil {
		return 0, err
	}
	if !done {
		return 0, errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	field := logrus.WithField("Service", svc.Name)

	svc.Lock()

	defer func() {
		state := statusServiceUsersUpdated
		if err != nil {
			field.Errorf("%+v", err)

			state = statusServiceUsersUpdateFailed
		}

		_err := svc.statusLock.SetStatus(state)
		if _err != nil {
			field.Errorf("%+v", err)
		}

		svc.Unlock()
	}()

	code := 200

	list, err := svc.getUsers()
	if err != nil {
		return 0, err
	}

	users := converteToUsers(svc.ID, req)
	update := make([]database.User, 0, len(req))
	addition := make([]database.User, 0, len(req))
	for i := range users {
		exist := false
		for u := range list {
			if list[u].Username == users[i].Username {
				users[i].ID = list[u].ID
				users[i].CreatedAt = list[u].CreatedAt

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
			return 0, errors.Wrap(err, "add user to switch manager")
		}
		logrus.Debug("add User:", swmUsers[i].UserName)
	}

	swmUsers = converteToSWM_Users(update)
	for i := range swmUsers {
		err := smlib.UptUser(addr, port, swmUsers[i])
		if err != nil {
			return 0, errors.Wrap(err, "update user to switch manager")
		}
		logrus.Debug("update User:", swmUsers[i].UserName)
	}

	err = database.TxUpdateUsers(addition, update)
	if err != nil {
		return 0, err
	}

	_, err = svc.reloadUsers()

	return code, err
}

// DeleteServiceUsers delete service users
func (svc *Service) DeleteServiceUsers(usernames []string, all bool) error {
	svc.Lock()
	defer svc.Unlock()

	users, err := svc.getUsers()
	if err != nil {
		return err
	}

	list := make([]database.User, 0, len(usernames))
	none := make([]string, 0, len(usernames))

	if all {
		list = users
	} else {

		for i := range usernames {
			exist := false

			for u := range users {

				if users[u].Username == usernames[i] {

					list = append(list, users[u])
					exist = true

					break
				}
			}

			if !exist {
				none = append(none, usernames[i])
			}
		}

		if len(none) > 0 {
			logrus.WithField("Service", svc.Name).Warnf("%s aren't service users", none)
		}
	}

	addr, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		return err
	}

	swmUsers := converteToSWM_Users(list)

	for i := range swmUsers {
		err := smlib.DelUser(addr, port, swmUsers[i])
		if err != nil {
			return errors.Wrapf(err, "delete service user in switchManager,Service=%s,user=%v", svc.Name, swmUsers[i])
		}
	}

	err = database.TxDeleteUsers(list)
	if err != nil {
		return err
	}

	svc.reloadUsers()

	return nil
}

func defaultServiceUsers(service string, sys database.Configurations) []database.User {
	now := time.Now()
	return []database.User{
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      User_Type_DB,
			Username:  sys.CheckUsername,
			Password:  sys.CheckPassword,
			Role:      _User_Check_Role,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      User_Type_DB,
			Username:  sys.MonitorUsername,
			Password:  sys.MonitorPassword,
			Role:      _User_Monitor_Role,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      User_Type_DB,
			Username:  sys.ApplicationUsername,
			Password:  sys.ApplicationPassword,
			Role:      _User_Application_Role,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      User_Type_DB,
			Username:  sys.DBAUsername,
			Password:  sys.DBAPassword,
			Role:      _User_DBA_Role,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      User_Type_DB,
			Username:  sys.DBUsername,
			Password:  sys.DBPassword,
			Role:      _User_DB_Role,
			ReadOnly:  false,
			CreatedAt: now,
		},
		database.User{
			ID:        utils.Generate32UUID(),
			ServiceID: service,
			Type:      User_Type_DB,
			Username:  sys.ReplicationUsername,
			Password:  sys.ReplicationPassword,
			Role:      _User_Replication_Role,
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
		case users[i].Type == User_Type_DB:
		case users[i].Type == User_Type_Proxy:

		case strings.ToLower(users[i].Type) == strings.ToLower(User_Type_DB):

			users[i].Type = User_Type_DB

		case strings.ToLower(users[i].Type) == strings.ToLower(User_Type_Proxy):

			users[i].Type = User_Type_Proxy

		default:
			logrus.WithField("Service", service).Warnf("skip:%s Role='%s'", users[i].Username, users[i].Type)
			continue
		}

		switch {
		case users[i].Role == _User_DB_Role:
		case users[i].Role == _User_Application_Role:
		case users[i].Role == _User_Check_Role:
		case users[i].Role == _User_DBA_Role:
		case users[i].Role == _User_Monitor_Role:
		case users[i].Role == _User_Replication_Role:

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_DB_Role):

			users[i].Role = _User_DB_Role

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_Application_Role):

			users[i].Role = _User_Application_Role

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
		case users[i].Type == User_Type_Proxy:
		case users[i].Type == User_Type_DB:

		case strings.ToLower(users[i].Type) == strings.ToLower(User_Type_DB):

			users[i].Type = User_Type_DB

		case strings.ToLower(users[i].Type) == strings.ToLower(User_Type_Proxy):

			users[i].Type = User_Type_Proxy

		default:
			logrus.WithField("Service", users[i].ServiceID).Warnf("skip:%s Type='%s'", users[i].Username, users[i].Type)
			continue
		}

		switch {
		case users[i].Role == _User_DB_Role:
		case users[i].Role == _User_Application_Role:

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_DB_Role):

			users[i].Role = _User_DB_Role

		case strings.ToLower(users[i].Role) == strings.ToLower(_User_Application_Role):

			users[i].Role = _User_Application_Role

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

func (svc *Service) getUsers() ([]database.User, error) {
	if len(svc.users) > 0 {
		return svc.users, nil
	}

	out, err := database.ListUsersByService(svc.ID, "")
	if err != nil {
		return nil, err
	}

	if len(out) > 0 {
		svc.users = out
	}

	return svc.users, nil
}

func (svc *Service) reloadUsers() ([]database.User, error) {
	out, err := database.ListUsersByService(svc.ID, "")
	if err != nil {
		return nil, err
	}

	svc.users = out

	return out, nil
}

func (svc *Service) getUserByRole(role string) (database.User, error) {
	users, err := svc.getUsers()
	if err != nil {
		return database.User{}, err
	}

	for i := range users {
		if users[i].Role == role {
			return users[i], nil
		}
	}

	users, err = svc.reloadUsers()
	if err != nil {
		return database.User{}, err
	}

	for i := range users {
		if users[i].Role == role {
			return users[i], nil
		}
	}

	return database.User{}, errors.New("not found Service User")
}

func (svc *Service) getUnit(nameOrID string) (*unit, error) {
	for _, u := range svc.units {
		if (u.ID == nameOrID || u.Name == nameOrID) && u != nil {
			return u, nil
		}
	}

	return nil, errors.Errorf("not found Unit '%s'", nameOrID)
}

func (gd *Gardener) addService(svc *Service) error {
	if svc == nil {
		return errors.New("Service cannot be nil pointer")
	}

	gd.RLock()
	for i := range gd.services {
		if gd.services[i].ID == svc.ID || gd.services[i].Name == svc.Name {
			gd.RUnlock()

			return errors.Errorf("Service %s existed", svc.Name)
		}
	}
	gd.RUnlock()

	gd.Lock()
	gd.services = append(gd.services, svc)
	gd.Unlock()

	return nil
}

// GetService returns Service of the Gardener
func (gd *Gardener) GetService(nameOrID string) (*Service, error) {
	gd.RLock()

	for i := range gd.services {
		if gd.services[i].ID == nameOrID || gd.services[i].Name == nameOrID {
			gd.RUnlock()

			return gd.services[i], nil
		}
	}

	gd.RUnlock()

	return gd.reloadService(nameOrID)
}

func (gd *Gardener) reloadService(nameOrID string) (*Service, error) {
	service, err := database.GetService(nameOrID)
	if err != nil {

		if errors.Cause(err) == sql.ErrNoRows {
			return nil, errors.Wrap(errServiceNotFound, "reload Service:"+nameOrID)
		}

		return nil, err
	}

	entry := logrus.WithField("Service", service.Name)

	units, err := database.ListUnitByServiceID(service.ID)
	if err != nil {
		return nil, err
	}

	svc := newService(service, len(units), int(gd.createRetry))
	svc.Lock()
	defer svc.Unlock()

	for i := range units {
		// rebuild units
		u, err := gd.reloadUnit(units[i])
		if err != nil {
			entry.WithField("Unit", units[i].Name).WithError(err).Error("reload unit")
		}

		svc.units = append(svc.units, &u)
	}

	strategies, err := database.ListBackupStrategyByServiceID(service.ID)
	if err != nil {
		entry.WithError(err).Warn("List Backup Strategy by ServiceID")
	}

	for i := range strategies {
		if strategies[i].Enabled &&
			strategies[i].Spec != manuallyBackupStrategy &&
			time.Now().Before(strategies[i].Valid) {

			svc.backup = &strategies[i]
			break
		}
	}

	if len(service.Desc) > 0 {
		err := json.Unmarshal([]byte(service.Desc), &svc.base)
		if err != nil {
			entry.WithError(err).Warn("JSON unmarshal Service.Description")
		}
	}

	svc.authConfig, err = gd.registryAuthConfig()
	if err != nil {
		entry.WithError(err).Error("Registry auth config")
	}

	_, err = svc.reloadUsers()
	if err != nil {
		entry.WithError(err).Error("list Users by serviceID:", service.ID)
	}

	entry.Debug("reload Service")

	exist := false
	gd.Lock()
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

	return svc, nil
}

// CreateService create new Service,create and start the Service
func (gd *Gardener) CreateService(req structs.PostServiceRequest) (*Service, string, string, error) {
	authConfig, err := gd.registryAuthConfig()
	if err != nil {
		logrus.Errorf("get Registry Auth Config:%+v", err)
		return nil, "", "", err
	}

	sys, err := gd.systemConfig()
	if err != nil {
		return nil, "", "", err
	}

	svc, task, err := buildService(req, authConfig, sys, int(gd.createRetry))
	if err != nil {
		logrus.WithError(err).Error("build Service")

		return svc, "", task.ID, err
	}

	strategyID := ""
	svc.RLock()
	if svc.backup != nil {
		strategyID = svc.backup.ID
	}
	svc.RUnlock()

	logrus.WithFields(logrus.Fields{
		"Servcie": svc.Name,
	}).Info("Service saved into database")

	err = gd.addService(svc)
	if err != nil {
		logrus.WithField("Service", svc.Name).WithError(err).Error("Service add to Gardener")

		return svc, strategyID, task.ID, err
	}

	background := func(context.Context) error {
		ok, val, err := svc.statusLock.CAS(statusServiceAllocating, func(val int) bool {
			return val == statusServcieBuilding
		})
		if err != nil {
			return err
		}
		if !ok {
			return errors.Errorf("Service %s status conflict,want %x but got %x", svc.Name, statusServcieBuilding, val)
		}

		svc.Lock()
		defer svc.Unlock()

		entry := logrus.WithField("Service", svc.Name)

		err = gd.serviceScheduler(svc, task)
		if err != nil {
			entry.Errorf("scheduler:%+v", err)

			return err
		}

		entry.Debugf("scheduler OK!")

		err = gd.serviceExecute(svc)
		if err != nil {
			entry.Errorf("execute:%+v", err)

			return err
		}

		if svc.backup != nil {
			bs := newBackupJob(svc)
			gd.registerBackupStrategy(bs)
		}

		return nil
	}

	updater := func(code int, msg string) error {
		return database.UpdateTaskStatus(task, int64(code), time.Now(), msg)
	}

	worker := NewAsyncTask(context.Background(),
		background,
		nil,
		updater,
		10*time.Minute)

	err = worker.Run()
	if err != nil {
		logrus.WithField("Service", svc.Name).Errorf("%+v", err)
	}

	return svc, strategyID, task.ID, err
}

// RebuildService rebuilds the nameOrID Service
func (gd *Gardener) RebuildService(nameOrID string) (*Service, string, string, error) {
	svc, err := gd.GetService(nameOrID)
	if errors.Cause(err) == errServiceNotFound {
		return nil, "", "", err
	}

	ok, status, err := svc.statusLock.CAS(statusServiceDeleting, isStatusFailure)
	if err != nil {
		return nil, "", "", err
	}

	if !ok {
		return nil, "", "", errors.Errorf("Service status conflict,%x is none of (%x,%x,%x)",
			status, statusServiceAllocateFailed, statusServiceContainerCreateFailed, statusServiceStartFailed)
	}

	desc := structs.PostServiceRequest{ID: svc.ID}

	if err := json.Unmarshal([]byte(svc.Desc), &desc); err != nil {
		return nil, "", "", errors.Wrap(err, "decode description failed")
	}

	err = gd.RemoveService(nameOrID, true, true, 0)
	if err != nil {
		return nil, "", "", err
	}

	return gd.CreateService(desc)
}

// StartService start service
func (svc *Service) StartService() (err error) {
	done, val, err := svc.statusLock.CAS(statusServiceStarting, isStatusNotInProgress)
	if err != nil {
		return err
	}
	if !done {
		return errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	field := logrus.WithField("Service", svc.Name)

	svc.Lock()

	defer func() {
		state := statusServiceStarted
		if err != nil {
			field.Errorf("%+v", err)

			state = statusServiceStartFailed
		}

		_err := svc.statusLock.SetStatus(state)
		if _err != nil {
			field.Errorf("%+v", _err)
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
		}

		_err := database.TxUpdateUnitStatus(&u.Unit, code, msg)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Unit":   u.Name,
				"Status": code,
				"Error":  msg,
			}).Errorf("update Unit %+v", _err)

			return err
		}
	}

	return nil
}

func (svc *Service) copyServiceConfig() error {
	for _, u := range svc.units {
		forbid, can := u.CanModify(u.configures)
		if !can {
			return errors.Errorf("forbid modifying service configs,%s", forbid)
		}

		defConfig, err := u.defaultUserConfig(svc, u)
		if err != nil {
			logrus.Errorf("Unit %s:%s,defaultUserConfig error,%s", u.Name, u.ImageName, err)

			return err
		}

		for key, val := range u.configures {
			defConfig[key] = val
		}

		err = u.copyConfig(defConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) initService() error {
	users, err := svc.getUsers()
	if err != nil {
		return err
	}

	var (
		args  = make([]string, 0, len(users)*3)
		funcs = make([]func() error, 0, len(svc.units))
	)

	for i := range users {
		if users[i].Type != User_Type_DB {
			continue
		}

		args = append(args, users[i].Role, users[i].Username, users[i].Password)
	}

	for i := range svc.units {
		u := svc.units[i]

		funcs = append(funcs, func() error {
			return u.initService(args...)
		})
	}

	err = GoConcurrency(funcs)
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

	return errors.Errorf("Service %s,status conflict:expected %d but got %d", svc.Name, expected, val)
}

func (svc *Service) statusCAS(expected, value int64) error {
	if atomic.CompareAndSwapInt64(&svc.Status, expected, value) {
		return nil
	}

	return errors.Errorf("Service %s,status conflict:expected %d but got %d", svc.Name, expected, svc.Status)
}

func (svc *Service) startService() error {
	var (
		swm   *unit
		funcs = make([]func() error, 0, len(svc.units))
	)

	for i := range svc.units {
		if svc.units[i].Type == _SwitchManagerType {
			swm = svc.units[i]
			continue
		}
		funcs = append(funcs, svc.units[i].startService)
	}

	err := GoConcurrency(funcs)
	if err != nil {
		logrus.WithField("Service", svc.Name).WithError(err).Error("start services")
		return err
	}

	if swm != nil {
		err = swm.startService()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Service": svc.Name,
				"Unit":    swm.Name,
			}).WithError(err).Error("start switch manager service")
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

// StopService stop the Service,only stop the upsql type unit service
func (svc *Service) StopService() (err error) {
	done, val, err := svc.statusLock.CAS(statusServiceStoping, isStatusNotInProgress)
	if err != nil {
		return err
	}
	if !done {
		return errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	field := logrus.WithField("Service", svc.Name)

	svc.Lock()

	defer func() {
		state := statusServiceStoped
		if err != nil {
			field.Errorf("%+v", err)

			state = statusServiceStopFailed
		}

		_err := svc.statusLock.SetStatus(state)
		if _err != nil {
			field.Errorf("%+v", _err)
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
			}).WithError(err).Error("stop switch manager service")

			_err := checkContainerError(err)
			if _err != errContainerNotRunning && _err != errContainerNotFound {
				return err
			}
		}
	}

	funcs := make([]func() error, 0, len(svc.units))
	for i := range svc.units {
		if svc.units[i].Type == _SwitchManagerType {
			continue
		}

		funcs = append(funcs, svc.units[i].stopService)
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
	funcs := make([]func() error, 0, len(svc.units))

	for i := range svc.units {
		if svc.units[i].Type == _SwitchManagerType {
			swm = svc.units[i]
			continue
		}

		funcs = append(funcs, svc.units[i].stopService)
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

func (svc *Service) removeContainers(force, rmVolumes bool) error {
	logrus.Debug(svc.Name, " remove Containers")
	for _, u := range svc.units {

		atomic.StoreInt64(&u.Status, statusUnitDeleting)

		logrus.Debug(u.Name, " remove Container")
		err := u.removeContainer(force, rmVolumes)
		if err != nil {
			logrus.Errorf("container %s remove,-f=%v -v=%v,error:%s", u.Name, force, rmVolumes, err)
			if errors.Cause(err) == errEngineIsNil {
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

// ModifyUnitConfig modify unit service config on live
func (svc *Service) ModifyUnitConfig(_type string, config map[string]interface{}) (err error) {
	done, val, err := svc.statusLock.CAS(statusServiceConfigUpdating, isStatusNotInProgress)
	if err != nil {
		return err
	}
	if !done {
		return errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	field := logrus.WithField("Service", svc.Name)

	svc.Lock()

	defer func() {
		state := statusServiceConfigUpdated
		if err != nil {
			field.Errorf("%+v", err)

			state = statusServiceConfigUpdateFailed
		}

		_err := svc.statusLock.SetStatus(state)
		if _err != nil {
			field.Errorf("%+v", err)
		}

		svc.Unlock()
	}()

	if _type == _ProxyType || _type == _SwitchManagerType {
		return svc.updateUnitConfig(_type, config)
	}

	if _type != _UpsqlType {
		return errors.Errorf("unsupported Type:'%s'", _type)
	}

	units, err := svc.getUnitByType(_type)
	if err != nil {
		return err
	}

	dba, found := database.User{}, false
	for i := range svc.users {
		if svc.users[i].Role == _User_DBA_Role {
			dba = svc.users[i]
			found = true
			break
		}
	}
	if !found {
		return errors.Errorf("Service %s missing User Role:%s", svc.Name, _User_DBA_Role)
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
			return errors.Errorf("cannot modify UnitConfig,key:%s", out)
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
			entry := logrus.WithField("Unit", u.Name)

			engine, _err := u.Engine()
			if _err != nil {
				entry.Warn(err)
				continue
			}

			for i := range originalCmds {

				_, _err := containerExec(context.Background(), engine, u.ContainerID, originalCmds[i][:], false)
				if _err != nil {
					entry.WithError(_err).Error("Rollback command modify")
				}
			}

			for _, u := range cnfRollback {
				u.parent.Content = copyContent

				_err := u.copyConfig(nil)
				if _err != nil {
					entry.WithError(_err).Error("Rollback config file modify")
				}
			}
		}
	}()

	err = u.configParser.ParseData([]byte(copyContent))
	if err != nil {
		return err
	}

	for key, val := range config {
		oldValue := u.configParser.String(key)
		cmds := template
		old := template
		parts := strings.SplitAfterN(key, "::", 2)
		if len(parts) == 2 {
			key = parts[1]
		} else if len(parts) == 1 {
			key = parts[0]
		}

		err = u.configParser.Set(key, fmt.Sprintf("%v", val))
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

			_, err := containerExec(context.Background(), engine, u.ContainerID, cmdList[i][:], false)
			if err != nil {
				return err
			}
		}
	}

	for _, u := range units {
		cnfRollback = append(cnfRollback, u)

		err = u.copyConfig(config)
		if err != nil {
			return err
		}
	}

	return nil
}

// updateUnitConfig update unit config
func (svc *Service) updateUnitConfig(_type string, config map[string]interface{}) error {
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
			return errors.Errorf("Illegal keys:%s,or keys unable to modified", keys)
		}

		err := u.copyConfig(config)
		if err != nil {
			return err
		}

		if u.MustRestart(config) {

			// disable restart Service for now
			logrus.WithField("Unit", u.Name).Warn("should restart service to make new config file works")
			return nil

			// err := u.stopService()
			// if err != nil {
			//  logrus.WithField("Unit", u.Name).WithError(err).Error("Stop service")
			// }

			// err = u.startService()
			// if err != nil {
			//  logrus.WithField("Unit", u.Name).WithError(err).Error("Start service")
			//  return err
			// }
		}
	}

	return nil
}

func swmInitTopology(svc *Service, swm *unit, users []database.User) error {
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
		swmUsers        = converteToSWM_Users(users)
	)
	for i := range users {
		if users[i].Role == _User_DBA_Role {
			dba = users[i]
		} else if users[i].Role == _User_Replication_Role {
			replicater = users[i]
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
		Users:               swmUsers,            //  []User   `json:"users"`
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

func (svc *Service) initTopology() error {
	swm, err := svc.getSwithManagerUnit()
	if err != nil {
		return err
	}

	users, err := svc.getUsers()
	if err != nil {
		return err
	}

	return swmInitTopology(svc, swm, users)
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

	if _type == _SwitchManagerType {
		return nil, errors.Wrap(ErrSwitchManagerUnitNotFound, "Service "+svc.Name)
	}

	return nil, errors.Errorf("Service:%s,not found any unit by type '%s'", svc.Name, _type)
}

func (svc *Service) getSwithManagerUnit() (*unit, error) {
	units, err := svc.getUnitByType(_SwitchManagerType)
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

// GetSwitchManagerAddr returns the Service switchManager unit address
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
		return addr, port, nil, errors.New("not found Master Unit")
	}

	master, err := svc.getUnit(masterName)

	return addr, port, master, err
}

// UnitIsolate isolate a unit
func (gd *Gardener) UnitIsolate(nameOrID string) error {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return err
	}

	service, err := gd.GetService(table.ServiceID)
	if err != nil {
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
		return err
	} else if u.Type == _SwitchManagerType {
		return errors.Errorf("unable to isolate Unit '%s:%s'", u.Type, u.Name)
	}

	ip, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"Service":  svc.Name,
		"Unit":     u.Name,
		"swm_IP":   ip,
		"swm_port": port,
	}).Debug("isolate unit")

	err = smlib.Isolate(ip, port, u.Name)

	return errors.Wrap(err, "isolate unit")
}

// UnitSwitchBack switchback a unit
func (gd *Gardener) UnitSwitchBack(nameOrID string) error {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return err
	}

	service, err := gd.GetService(table.ServiceID)
	if err != nil {
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
		return err
	} else if u.Type == _SwitchManagerType {
		return errors.Errorf("unable to switchback Unit '%s:%s'", u.Type, u.Name)
	}

	ip, port, err := svc.getSwitchManagerAddr()
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"Service":  svc.Name,
		"Unit":     u.Name,
		"swm_IP":   ip,
		"swm_port": port,
	}).Debug("switchback unit")

	err = smlib.Recover(ip, port, u.Name)

	return errors.Wrap(err, "switchback unit")
}

// TemporaryServiceBackupTask execute a temporary backup task
func (gd *Gardener) TemporaryServiceBackupTask(service, nameOrID string) (_ string, err error) {
	if nameOrID != "" {
		u, err := database.GetUnit(nameOrID)
		if err != nil {
			logrus.WithField("Unit", nameOrID).WithError(err).Error("not found Unit")
			return "", err
		}

		if service == "" {
			service = u.ServiceID
		}
	}

	svc, err := gd.GetService(service)
	if err != nil {

		return "", err
	}

	done, val, err := svc.statusLock.CAS(statusServiceBackuping, isStatusNotInProgress)
	if err != nil {
		return "", err
	}
	if !done {
		return "", errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	defer func() {
		if err != nil {
			_err := svc.statusLock.SetStatus(statusServiceBackupFailed)
			if _err != nil {
				err = errors.Errorf("%+v\n%+v", err, _err)
			}
		}
	}()

	ok, err := checkBackupFiles(svc.ID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.Errorf("Service %s:no more space for backup task", svc.Name)
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
		return "", err
	}

	if backup == nil {
		backup = master
	}

	sys, err := gd.systemConfig()
	if err != nil {
		logrus.WithError(err).Errorf("get SystemConfig")
		return "", err
	}

	if state := gd.Containers().Get(master.Name).State; state != "running" {
		return "", errors.Errorf("Unit %s is busy,container Status=%s", master.Name, state)
	}

	now := time.Now()
	strategy := database.BackupStrategy{
		ID:        utils.Generate64UUID(),
		Name:      backup.Name + "_backup_manually_" + utils.TimeToString(now),
		Type:      "full",
		ServiceID: svc.ID,
		Spec:      manuallyBackupStrategy,
		Valid:     now,
		Enabled:   false,
		BackupDir: sys.BackupDir,
		Timeout:   2 * 60 * 60,
		CreatedAt: now,
	}

	task := database.NewTask(backup.Name, backupManualTask, backup.ID, "", nil, strategy.Timeout)

	entry := logrus.WithFields(logrus.Fields{
		"Unit":     backup.Name,
		"Strategy": strategy.ID,
		"Task":     task.ID,
	})

	creater := func() error {
		task.Status = statusTaskCreate
		err = database.TxInsertBackupStrategyAndTask(strategy, task)

		return err
	}

	update := func(code int, msg string) error {
		task.Status = int64(code)

		err := database.TxUpdateUnitStatusWithTask(&backup.Unit, &task, msg)
		if err != nil {
			entry.WithError(err).Errorf("Update TaskStatus code=%d,message=%s", code, msg)
		}

		return err
	}

	background := func(ctx context.Context) (err error) {
		defer func() {
			state := statusServiceBackupDone
			if err != nil {
				state = statusServiceBackupFailed
			}

			_err := svc.statusLock.SetStatus(state)
			if _err != nil {
				err = errors.Errorf("%+v\n%+v", err, _err)
			}

			_err = smlib.UnLock(addr, port)
			if _err != nil {
				entry.Errorf("unlock switch_manager %s:%d:%s", addr, port, _err)
				err = errors.Wrap(err, _err.Error())
			}
		}()

		args := []string{hostAddress + ":" + httpPort + "/v1.0/tasks/backup/callback",
			task.ID, strategy.ID, backup.ID, strategy.Type, strategy.BackupDir}

		return backup.backup(ctx, args...)
	}

	worker := NewAsyncTask(context.Background(),
		background,
		creater,
		update,
		time.Duration(strategy.Timeout)*time.Second)

	err = worker.Run()

	return task.ID, err
}

type pendingContainerUpdate struct {
	containerID string
	cpusetCpus  string
	unit        *unit
	svc         *Service
	engine      *cluster.Engine
	config      container.UpdateConfig
}

// ServiceScale scale assigned type units cpu\memory\volumes resources
func (gd *Gardener) ServiceScale(name string, scale structs.PostServiceScaledRequest) error {
	svc, err := gd.GetService(name)
	if err != nil {
		return err
	}

	err = validServiceScale(svc, scale)
	if err != nil {
		return err
	}

	done, val, err := svc.statusLock.CAS(statusServiceScaling, isStatusNotInProgress)
	if err != nil {
		return err
	}
	if !done {
		return errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	go gd.serviceScale(svc, scale)

	return nil
}

func (gd *Gardener) serviceScale(svc *Service,
	scale structs.PostServiceScaledRequest) (err error) {

	field := logrus.WithFields(logrus.Fields{
		"Service": svc.Name,
		"Type":    scale.Type,
	})

	var storePendings []*pendingAllocStore

	svc.Lock()
	gd.scheduler.Lock()

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}

		status := statusServiceScaled

		if err == nil {
			err = svc.updateDescAfterScale(scale)
		} else {
			field.Errorf("Service scale,%+v\n%+v", scale, err)

			status = statusServiceScaleFailed

			_err := cancelStoreExtend(storePendings)
			if _err != nil {
				err = errors.Errorf("%+v\n%+v", err, _err)
			}
		}

		_err := svc.statusLock.SetStatus(status)
		if _err != nil {
			field.Errorf("%+v", _err)
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
		return err
	}

	for i := range pendings {
		if pendings[i].svc == nil {
			pendings[i].svc = svc
		}
		err = pendings[i].containerUpdate()
		if err != nil {
			return err
		}
	}

	for _, pending := range storePendings {
		for _, lv := range pending.localStore {
			err = localVolumeExtend(pending.unit.engine.IP, lv)
			if err != nil {
				field.WithField("Unit", pending.unit.Name).Errorf("update volume error:\n%+v", err)
				return err
			}
			field.WithField("Unit", pending.unit.Name).Debugf("update volume %v", lv)
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
	err = p.unit.copyConfig(defConfig)
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
		if svc.Desc == "" {
			table, err := database.GetService(svc.ID)
			if err != nil {
				return nil, err
			}
			svc.Service = table
		}

		if svc.Desc != "" {
			err := json.NewDecoder(strings.NewReader(svc.Desc)).Decode(svc.base)
			if err != nil {
				return nil, errors.Wrap(err, "JSON decode Service.Desc")
			}
		}
	}

	if svc.base == nil {
		return nil, errors.Errorf("Service %s with null Description", svc.Name)
	}

	return svc.base, nil
}

func (svc *Service) updateDescAfterScale(scale structs.PostServiceScaledRequest) error {
	dsp, err := svc.getServiceDescription()
	if err != nil {
		return err
	}

	des := *dsp

	if scale.UpdateConfig != nil {
		des.UpdateModuleConfig(scale.Type, *scale.UpdateConfig)
	}

	if len(scale.Extensions) > 0 {
		des.UpdateModuleStore(scale.Type, scale.Extensions)
	}

	des.BackupMaxSize += scale.ExtendBackup

	buffer := bytes.NewBuffer(nil)
	err = json.NewEncoder(buffer).Encode(&des)
	if err != nil {
		return err
	}

	desc := buffer.String()
	err = database.UpdateServcieDesc(svc.ID, desc, des.BackupMaxSize)
	if err != nil {
		return err
	}

	svc.Desc = desc
	svc.base = &des

	return nil
}

// RemoveService remove the assigned Service from the Gardener
func (gd *Gardener) RemoveService(nameOrID string, force, volumes bool, timeout int) (err error) {
	entry := logrus.WithFields(logrus.Fields{
		"Service": nameOrID,
		"force":   force,
		"volumes": volumes,
	})
	entry.Info("Removing Service...")

	svc, err := gd.reloadService(nameOrID)
	if err != nil {
		if errors.Cause(err) == errServiceNotFound {
			return nil
		}
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
		if err != nil {
			_err := svc.statusLock.SetStatus(statusServiceDeleteFailed)
			if _err != nil {
				err = errors.Wrap(err, _err.Error())
			}
		}
	}()

	err = svc.statusLock.SetStatus(statusServiceDeleting)
	if err != nil {
		return err
	}

	entry.Debug("Service Delete... stop service & stop containers & rm containers & deregister")

	err = svc.delete(gd, force, volumes, true, timeout)
	if err != nil {
		return err
	}

	// delete database records relation svc.ID
	entry.Debug("Detele Service Relation...")
	err = database.DeteleServiceRelation(svc.ID, volumes)
	if err != nil {
		entry.Errorf("DeteleServiceRelation error:%+v", err)
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
		err = gd.removeCronJob(svc.backup.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) delete(gd *Gardener, force, rmVolumes, recycle bool, timeout int) error {
	svc.Lock()
	defer svc.Unlock()

	entry := logrus.WithField("Service", svc.Name)

	funcs := make([]func() error, 0, len(svc.units))

	for i := range svc.units {
		u := svc.units[i]

		if force {
			funcs = append(funcs, u.kill)
			continue
		}

		f := func() error {
			if _, err := u.Engine(); errors.Cause(err) == errEngineIsNil {
				return nil
			}

			entry.WithField("Unit", u.Name).Debug("stop unit service")

			err := u.forceStopService()
			if err != nil &&
				u.container.Info.State.Running &&
				err.Error() != "EOF" {

				return errors.Wrapf(err, "%s force stop Service error", u.Name)
			}

			entry.WithField("Unit", u.Name).Debug("stop container")

			err = u.forceStopContainer(timeout)
			if err != nil {
				if !u.container.Info.State.Running {
					return nil
				}

				if err.Error() == "EOF" {
					return nil
				}
				err = errors.Wrapf(err, "%s force stop container error", u.Name)
			}

			return err
		}

		funcs = append(funcs, f)
	}

	err := GoConcurrency(funcs)
	if err != nil {
		entry.WithError(err).Error("stop Service")

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

	for _, u := range svc.units {

		lvs, err := database.ListVolumesByUnitID(u.ID)
		if err != nil {
			entry.WithField("Unit", u.Name).Errorf("%+v", err)
			continue
		}

		if rmVolumes {
			// remove volumes
			for i := range lvs {
				found, err := gd.RemoveVolumes(lvs[i].Name)
				if err != nil {
					entry.Warnf("Remove volume=%s,found=%t,error=%+v", lvs[i].Name, found, err)

					v := gd.Volumes().Get(lvs[i].Name)
					if v != nil {
						_err := v.Engine.RefreshVolumes()
						if _err != nil {
							return errors.Wrap(_err, "Engine refresh volumes,Addr:"+v.Engine.Addr)
						}

						v = v.Engine.Volumes().Get(lvs[i].Name)
						if v == nil {
							continue
						}

						_err = v.Engine.RemoveVolume(lvs[i].Name)
						if _err == nil {
							continue
						} else {
							return errors.Wrap(_err, "Engine remove volume,Addr:"+v.Engine.Addr)

						}
					}

					return err
				}
				entry.Debug(i, len(lvs), "Remove volume ", lvs[i].Name)
			}
		}

		//		if recycle {
		//			for i := range lvs {

		//				luns, err := database.ListLUNByVgName(lvs[i].VGName)
		//				if err != nil {
		//					return err
		//				}
		//			}
		//		}
	}

	err = svc.deregisterInHorus()
	if err != nil {
		entry.Errorf("deregister in Horus error:%+v", err)
	}

	err = svc.deregisterServices()
	if err != nil {
		entry.Errorf("deregister in consul error:%+v", err)
	}

	err = deleteConsulKVTree(svc.ID)
	if err != nil {
		entry.Errorf("delete consul KV:%s,%+v", svc.ID, err)
	}

	return nil
}

var (
	errContainerNotFound   = stderrors.New("no such container")
	errContainerNotRunning = stderrors.New("container not running")
)

func checkContainerError(err error) error {
	if err == nil {
		return nil
	}

	if err == errContainerNotRunning || err == errContainerNotFound {
		return err
	}

	if _err := errors.Cause(err); _err == errContainerNotRunning || _err == errContainerNotFound {
		return _err
	}

	if strings.Contains(err.Error(), "No such container") {
		return errContainerNotFound
	}

	if match, _ := regexp.MatchString(`Container \S* is not running`, err.Error()); match {
		return errContainerNotRunning
	}

	return err
}
