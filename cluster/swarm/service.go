package swarm

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/samalba/dockerclient"
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
	authConfig *dockerclient.AuthConfig
}

func NewService(svc database.Service, retry, unitNum, userNum int) *Service {
	return &Service{
		Service:           svc,
		failureRetry:      retry,
		units:             make([]*unit, unitNum),
		users:             make([]database.User, userNum),
		pendingContainers: make(map[string]*pendingContainer),
	}
}

func BuildService(req structs.PostServiceRequest) (*Service, error) {
	if err := Validate(req); err != nil {
		return nil, err
	}

	svc := database.Service{}

	return NewService(svc, 2, 0, 0), nil
}

func Validate(req structs.PostServiceRequest) error {
	return nil
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

	gd.RLock()

	s, err := gd.GetService(svc.ID)

	gd.RUnlock()

	if s != nil || err == nil {

		return errors.New("Service exist")
	}

	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return errors.New("Service Status Conflict")
	}

	if err := svc.Insert(); err != nil {
		atomic.StoreInt64(&svc.Status, 0)

		return err
	}

	gd.Lock()
	gd.services = append(gd.services, svc)
	gd.Unlock()

	return nil
}

func (gd *Gardener) GetService(IDOrName string) (*Service, error) {
	for i := range gd.services {
		if gd.services[i].ID == IDOrName ||
			gd.services[i].Name == IDOrName {
			return gd.services[i], nil
		}
	}

	return nil, errors.New("Service Not Found")
}

func (svc *Service) CreateContainers() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
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

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) StartContainers() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].startContainer()
		if err != nil {
			return err
		}
	}

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) CopyServiceConfig() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
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

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) StartService() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].startService()
		if err != nil {
			return err
		}
	}

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) StopContainers() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].stopContainer(0)
		if err != nil {
			return err
		}
	}

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) StopService() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].stopService()
		if err != nil {
			return err
		}
	}

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) RemoveContainers() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].removeContainer(false, false)
		if err != nil {
			return err
		}
	}

	atomic.StoreInt64(&svc.Status, 1)

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
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].RegisterHealthCheck(nil)
		if err != nil {
			return err
		}
	}

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) DeregisterServices() (err error) {
	if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
		return nil
	}

	svc.Lock()

	defer func() {
		if r := recover(); r != nil || err != nil {
			atomic.StoreInt64(&svc.Status, 1)
		}

		svc.Unlock()
	}()

	for i := range svc.units {
		err = svc.units[i].DeregisterHealthCheck(nil)
		if err != nil {
			return err
		}
	}

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

func (svc *Service) Destroy() error {
	err := svc.StopService()
	if err != nil {
		return err
	}

	err = svc.StopContainers()
	if err != nil {
		return err
	}

	err = svc.RemoveContainers()
	if err != nil {
		return err
	}

	err = svc.DeregisterServices()
	if err != nil {
		return err
	}

	atomic.StoreInt64(&svc.Status, 1)

	return nil
}

// GetRoles returns role of units belong to Service,get infomation from consul server,
// map[unitID]role
func (svc *Service) getRoles() (map[string]string, error) {

	return nil, nil
}

func (svc *Service) GetUnitRole(role string) (*unit, error) {
	svc.RLock()
	defer svc.RUnlock()

	roles, err := svc.getRoles()
	if err != nil {
		return nil, err
	}

	for id, r := range roles {
		if role == r {
			return svc.getUnit(id)
		}
	}

	return nil, fmt.Errorf("Not Found unit role:%s In Service %s", role, svc.ID)

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

func (svc *Service) GetSwithManagerAddr() (addr string, port int, err error) {
	svc.RLock()
	u, err := svc.getUnitByType("Switch Manager")
	svc.RUnlock()

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
		if u.ports[i].Name == "Http Service" {
			port = u.ports[i].Port

			return addr, port, nil
		}
	}

	return addr, port, fmt.Errorf("Not Found")
}
