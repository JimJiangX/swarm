package garden

import (
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/scheduler"
	"github.com/docker/swarm/scheduler/filter"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/scheduler/strategy"
)

type Allocator interface {
	ListCandidates(clusters, filters []string, _type string) ([]database.Node, error)

	AlloctMemoryCPU(node *node.Node, cpu, memory int) (string, error)

	AlloctVolumes(string, *node.Node) ([]volume.VolumesCreateBody, error)

	AlloctNetworking(id, _type string, num int) (string, error)

	RecycleResource() error
}

type Garden struct {
	sync.Mutex
	ormer database.Ormer

	allocator   Allocator
	cluster     cluster.Cluster
	scheduler   *scheduler.Scheduler
	authConfig  *types.AuthConfig
	eventHander eventHander
}

func NewGarden(cluster cluster.Cluster, scheduler *scheduler.Scheduler, ormer database.Ormer, allocator Allocator, authConfig *types.AuthConfig) *Garden {
	return &Garden{
		// Mutex:       &scheduler.Mutex,
		allocator:   allocator,
		cluster:     cluster,
		ormer:       ormer,
		authConfig:  authConfig,
		eventHander: eventHander{ormer},
	}
}

func (gd *Garden) BuildService() (*Service, error) {
	svc := database.Service{}

	units := []database.Unit{}

	err := gd.ormer.InsertService(svc, units, nil, nil)
	if err != nil {
		return nil, err
	}

	service := newService(svc, gd.ormer, gd.cluster)
	service.units = units

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

type scheduleOption struct {
	nodes struct {
		clusterType string
		clusters    []string
		filters     []string
	}

	scheduler struct {
		strategy string
		filters  []string
	}
}

func (gd *Garden) schedule(config *cluster.ContainerConfig, opts scheduleOption) ([]*node.Node, error) {
	_scheduler := gd.scheduler

	if opts.scheduler.strategy != "" && len(opts.scheduler.filters) > 0 {
		strategy, _ := strategy.New(opts.scheduler.strategy)
		filters, _ := filter.New(opts.scheduler.filters)
		_scheduler = scheduler.New(strategy, filters)
	}

	if len(opts.nodes.filters) > 0 {
		config.AddConstraint("node!=" + strings.Join(opts.nodes.filters, "|"))
	}

	out, err := gd.allocator.ListCandidates(opts.nodes.clusters, opts.nodes.filters, opts.nodes.clusterType)
	if err != nil {
		return nil, err
	}

	engines := make([]string, 0, len(out))
	for i := range out {
		if out[i].EngineID != "" {
			engines = append(engines, out[i].EngineID)
		}
	}

	list := gd.cluster.ListEngines(engines...)
	nodes := make([]*node.Node, 0, len(list))
	for i := range list {
		n := node.NewNode(list[i])
		nodes = append(nodes, n)
	}

	nodes, err = _scheduler.SelectNodesForContainer(nodes, config)

	return nodes, err
}

type pendingUnit struct {
	database.Unit
	swarmID string

	config      *cluster.ContainerConfig
	networkings []string
	volumes     []volume.VolumesCreateBody
}

func (gd *Garden) Allocation(units []database.Unit) ([]pendingUnit, error) {
	config := cluster.BuildContainerConfig(container.Config{}, container.HostConfig{}, network.NetworkingConfig{})

	ncpu, memory := 1, 2<<20

	gd.Lock()
	defer gd.Unlock()

	nodes, err := gd.schedule(config, scheduleOption{})
	if err != nil {
		return nil, err
	}

	count := len(units)

	ready, bad := make([]pendingUnit, 0, count), make([]pendingUnit, 0, count)

	defer func() {
		// cancel allocation
		if len(bad) > 0 {
			err := gd.allocator.RecycleResource()
			if err != nil {
				// TODO:
			}

			for i := range bad {
				gd.cluster.RemovePendingContainer(bad[i].swarmID)
			}
		}
	}()

	for n := range nodes {
		pending := pendingUnit{
			Unit:        units[count-1],
			config:      resetContainerConfig(config),
			networkings: make([]string, 0, len(units)),
			volumes:     make([]volume.VolumesCreateBody, 0, len(units)),
		}

		cpuset, err := gd.allocator.AlloctMemoryCPU(nodes[n], ncpu, memory)
		if err != nil {
			continue
		}

		volumes, err := gd.allocator.AlloctVolumes(pending.Unit.ID, nodes[n])
		if len(volumes) > 0 {
			pending.volumes = append(pending.volumes, volumes...)
		}
		if err != nil {
			bad = append(bad, pending)
			continue
		}

		id, err := gd.allocator.AlloctNetworking(pending.Unit.ID, "networkingType", 1)
		if len(id) > 0 {
			pending.networkings = append(pending.networkings, id)
		}
		if err != nil {
			bad = append(bad, pending)
			continue
		}

		pending.swarmID = "random ID"
		pending.config.SetSwarmID(pending.swarmID)
		pending.Unit.EngineID = nodes[n].ID

		pending.config.HostConfig.Resources.CpusetCpus = cpuset
		pending.config.HostConfig.Resources.Memory = int64(memory)

		gd.cluster.AddPendingContainer(pending.Name, pending.swarmID, nodes[n].ID, pending.config)

		ready = append(ready, pending)
		if count--; count == 0 {
			break
		}
	}

	if count > 0 {
		bad = append(bad, ready...)
	}

	return ready, err
}

func resetContainerConfig(config *cluster.ContainerConfig) *cluster.ContainerConfig {
	// TODO:clone
	return config
}
