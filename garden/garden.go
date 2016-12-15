package garden

import (
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/scheduler"
	"github.com/docker/swarm/scheduler/filter"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/scheduler/strategy"
)

type Divider interface {
	ListCandidates(config cluster.ContainerConfig, scheduler *scheduler.Scheduler, clusters, filters []string, _type string) ([]*node.Node, error)
	DividVolumes(node.Node) ([]volume.VolumesCreateBody, error)
	DividNetworking(node.Node) error
}

type Garden struct {
	*sync.Mutex
	ormer database.Ormer

	divider     Divider
	cluster     cluster.Cluster
	scheduler   *scheduler.Scheduler
	authConfig  *types.AuthConfig
	eventHander eventHander
}

func NewGarden(cluster cluster.Cluster, scheduler *scheduler.Scheduler, ormer database.Ormer, divider Divider, authConfig *types.AuthConfig) *Garden {
	return &Garden{
		Mutex:       &scheduler.Mutex,
		divider:     divider,
		cluster:     cluster,
		ormer:       ormer,
		authConfig:  authConfig,
		eventHander: eventHander{ormer},
	}
}

func (gd *Garden) BuildService() (*Service, error) {
	svc := database.Service{}

	err := gd.ormer.InsertService(svc, nil, nil)
	if err != nil {
		return nil, err
	}

	service := newService(svc, gd.ormer, gd.cluster)

	return service, nil
}

func (gd *Garden) Service(nameOrID string) (*Service, error) {
	svc, err := gd.ormer.GetService(nameOrID)
	if err != nil {
		return nil, err
	}

	s := newService(svc, gd.ormer, gd.cluster)

	return s, nil
}

func (gd *Garden) Scheduler() ([]*node.Node, error) {
	config := cluster.ContainerConfig{}

	clusterType := "cluster type"

	clusters := []string{"clsuter1", "cluster2"}

	filters := []string{}

	schedl := gd.scheduler

	{
		sche := struct {
			Filters  []string
			Strategy string
		}{}

		if sche.Strategy != "" && len(sche.Filters) > 0 {
			strategy, _ := strategy.New(sche.Strategy)
			filters, _ := filter.New(sche.Filters)
			schedl = scheduler.New(strategy, filters)
		}
	}

	nodes, err := gd.divider.ListCandidates(config, schedl, clusters, filters, clusterType)

	return nodes, err
}
