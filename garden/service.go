package garden

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type Service struct {
	sl      statusLock
	svc     database.Service
	so      database.ServiceOrmer
	cluster cluster.Cluster
}

func newService(svc database.Service, so database.ServiceOrmer, cluster cluster.Cluster) *Service {
	return &Service{
		svc:     svc,
		so:      so,
		cluster: cluster,
		sl:      newStatusLock(svc.ID, so),
	}
}

func (svc *Service) getUnit(nameOrID string) (unit, error) {
	u := database.Unit{}

	return unit{u: u}, nil
}

func (svc *Service) Create() error {

	return nil
}

func (svc *Service) Start() error {
	return nil
}

func (svc *Service) Stop() error {
	return nil
}

func (svc *Service) Scale() error {
	return nil
}

func (svc *Service) Update() error {
	return nil
}

func (svc *Service) UpdateConfig() error {
	return nil
}

func (svc *Service) Exec() error {
	return nil
}

func (svc *Service) Remove() error {
	return nil
}
