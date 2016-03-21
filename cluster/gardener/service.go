package gardener

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/docker/swarm/cluster/gardener/database"
	"github.com/samalba/dockerclient"
)

type Service struct {
	sync.RWMutex

	failureRetry int

	database.Service
	base *PostServiceRequest

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

func BuildService(svc database.Service) (*Service, error) {
	if err := Validate(svc); err != nil {
		return nil, err
	}

	return NewService(svc, 2, 0, 0), nil
}

func Validate(svc database.Service) error {
	return nil
}

func (svc *Service) getUnit(name string) (*unit, error) {
	for i := range svc.units {
		if svc.units[i].Name == name {
			return svc.units[i], nil
		}
	}

	return nil, fmt.Errorf("Unit Not Found,%s", name)
}

func (region *Region) AddService(svc *Service) error {
	if svc == nil {
		return errors.New("Service Cannot be nil")
	}

	region.RLock()

	s, err := region.GetService(svc.ID)

	region.RUnlock()

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

	region.Lock()
	region.services = append(region.services, svc)
	region.Unlock()

	return nil
}

func (region *Region) GetService(IDOrName string) (*Service, error) {
	for i := range region.services {
		if region.services[i].ID == IDOrName ||
			region.services[i].Name == IDOrName {
			return region.services[i], nil
		}
	}

	return nil, errors.New("Service Not Found")
}
