package gardener

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/docker/swarm/cluster/gardener/database"
)

type Service struct {
	sync.RWMutex

	failureRetry int

	database.Service

	cluster *Cluster

	units  []*unit
	users  []database.User
	backup *database.BackupStrategy
}

func NewService(cl *Cluster, svc database.Service, retry, unitNum, userNum int) *Service {
	return &Service{
		cluster:      cl,
		Service:      svc,
		failureRetry: retry,
		units:        make([]*unit, unitNum),
		users:        make([]database.User, userNum),
	}
}

func BuildService(cl *Cluster, svc database.Service) (*Service, error) {
	if err := Validate(svc); err != nil {
		return nil, err
	}

	return NewService(cl, svc, 2, 0, 0), nil
}

func Validate(svc database.Service) error {
	return nil
}

func (c *Cluster) AddService(svc *Service) error {
	if svc == nil {
		return errors.New("Service Cannot be nil")
	}

	c.RLock()

	s, err := c.getService(svc.ID)

	c.RUnlock()

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

	c.Lock()
	c.services = append(c.services, svc)
	c.Unlock()

	return nil
}

func (c *Cluster) getService(IDOrName string) (*Service, error) {
	for i := range c.services {
		if c.services[i].ID == IDOrName ||
			c.services[i].Name == IDOrName {
			return c.services[i], nil
		}
	}

	return nil, errors.New("Service Not Found")
}
