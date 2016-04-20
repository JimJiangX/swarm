package swarm

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	"github.com/yiduoyunQ/smlib"
)

var (
	ErrServiceNotFound = errors.New("Service Not Found")
)

type Service struct {
	sync.RWMutex

	failureRetry int

	database.Service
	base *structs.PostServiceRequest

	pendingContainers map[string]*pendingContainer

	units      []*unit
	users      []database.User
	backup     *database.BackupStrategy
	task       *database.Task
	authConfig *types.AuthConfig
}

func NewService(svc database.Service, retry, unitNum int) *Service {
	return &Service{
		Service:           svc,
		failureRetry:      retry,
		units:             make([]*unit, unitNum),
		pendingContainers: make(map[string]*pendingContainer),
	}
}

func BuildService(req structs.PostServiceRequest, authConfig *types.AuthConfig) (*Service, error) {
	if warnings := Validate(req); len(warnings) > 0 {
		return nil, errors.New(strings.Join(warnings, ","))
	}

	strategy := &database.BackupStrategy{
		ID:          utils.Generate64UUID(),
		Type:        req.Strategy.Type,
		Spec:        req.Strategy.Spec,
		Valid:       req.Strategy.Valid,
		Enabled:     true,
		BackupDir:   req.Strategy.BackupDir,
		MaxSizeByte: req.Strategy.MaxSize,
		Retention:   req.Strategy.Retention * time.Second,
		Timeout:     req.Strategy.Timeout * time.Second,
		CreatedAt:   time.Now(),
	}

	svc := database.Service{
		ID:               utils.Generate64UUID(),
		Name:             req.Name,
		Description:      req.Description,
		Architecture:     req.Architecture,
		AutoHealing:      req.AutoHealing,
		AutoScaling:      req.AutoScaling,
		HighAvailable:    req.HighAvailable,
		Status:           _StatusServiceInit,
		BackupSpaceByte:  req.Strategy.MaxSize,
		BackupStrategyID: strategy.ID,
		CreatedAt:        time.Now(),
	}

	arch, err := getServiceArch(req.Architecture)
	if err != nil {
		return nil, err
	}

	nodeNum := 0
	for _, n := range arch {
		nodeNum += n
	}

	service := NewService(svc, 2, nodeNum)

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
		return nil, err
	}

	return service, nil
}

func Validate(req structs.PostServiceRequest) []string {
	_, err := getServiceArch(req.Architecture)
	if err != nil {
		return []string{err.Error()}
	}

	return nil
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
		if svc.units[i].ID == IDOrName ||
			svc.units[i].Name == IDOrName {
			return svc.units[i], nil
		}
	}

	return nil, fmt.Errorf("Unit Not Found,%s", IDOrName)
}

func (gd *Gardener) AddService(svc *Service) error {
	if svc == nil {
		return errors.New("Service Cannot be nil")
	}

	s, err := gd.GetService(svc.ID)

	if s != nil || err == nil {

		return errors.New("Service exist")
	}

	gd.Lock()
	gd.services = append(gd.services, svc)
	gd.Unlock()

	return nil
}

func (svc *Service) SaveToDB() error {
	return database.TxSaveService(&svc.Service, svc.backup, svc.task, svc.users)
}

func (gd *Gardener) GetService(IDOrName string) (*Service, error) {
	gd.RLock()

	for i := range gd.services {
		if gd.services[i].ID == IDOrName ||
			gd.services[i].Name == IDOrName {
			gd.RUnlock()

			return gd.services[i], nil
		}
	}

	gd.RUnlock()

	return nil, ErrServiceNotFound
}

func (gd *Gardener) CreateService(req structs.PostServiceRequest) (*Service, error) {
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		return nil, err
	}

	svc, err := BuildService(req, authConfig)
	if err != nil {
		return nil, err
	}

	if err := gd.AddService(svc); err != nil {
		return svc, err
	}

	if err := gd.ServiceToScheduler(svc); err != nil {
		return svc, err
	}

	if svc.backup != nil {
		bs := NewBackupJob(gd.host, svc)
		gd.RegisterBackupStrategy(bs)
	}

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

		err = u.CopyConfig(map[string]interface{}{})
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

		if u.Type == "mysql" {
			err := containerExec(u.engine, u.ContainerID, cmd, false)
			if err != nil {

			}
		}

	}

	for i := range svc.units {
		u := svc.units[i]

		if u.Type == "swith manager" {
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

		if u.Type == "swith manager" {

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

		if u.Type == "swith manager" {

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
		if u.networkings[i].Type == ContainersNetworking {
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
			if strings.EqualFold(node.Type, "master") {
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

	return nil
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
