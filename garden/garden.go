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
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/scheduler"
	"github.com/docker/swarm/scheduler/filter"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/scheduler/strategy"
)

const clusterLabel = "Cluster"

type allocator interface {
	ListCandidates(clusters, filters []string, _type string, stores []structs.VolumeRequire) ([]database.Node, error)

	AlloctCPUMemory(node *node.Node, cpu, memory int, reserved []string) (string, error)

	AlloctVolumes(id string, n *node.Node, stores []structs.VolumeRequire) ([]volume.VolumesCreateBody, error)

	AlloctNetworking(id, _type string, num int) (string, error)

	RecycleResource() error
}

type Garden struct {
	sync.Mutex
	ormer    database.Ormer
	kvClient kvstore.Client

	allocator   allocator
	cluster     cluster.Cluster
	scheduler   *scheduler.Scheduler
	authConfig  *types.AuthConfig
	eventHander cluster.EventHandler
}

func NewGarden(kvc kvstore.Client, cluster cluster.Cluster, scheduler *scheduler.Scheduler, ormer database.Ormer, allocator allocator, authConfig *types.AuthConfig) *Garden {
	gd := &Garden{
		// Mutex:       &scheduler.Mutex,
		kvClient:    kvc,
		allocator:   allocator,
		cluster:     cluster,
		ormer:       ormer,
		authConfig:  authConfig,
		eventHander: eventHander{ci: ormer},
	}

	err := cluster.RegisterEventHandler(gd.eventHander)
	if err != nil {
	}

	return gd
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
	highAvailable bool

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

func (gd *Garden) schedule(config *cluster.ContainerConfig, opts scheduleOption, stores []structs.VolumeRequire) ([]*node.Node, error) {
	_scheduler := gd.scheduler

	if opts.scheduler.strategy != "" && len(opts.scheduler.filters) > 0 {
		strategy, _ := strategy.New(opts.scheduler.strategy)
		filters, _ := filter.New(opts.scheduler.filters)
		_scheduler = scheduler.New(strategy, filters)
	}

	if len(opts.nodes.filters) > 0 {
		config.AddConstraint("node!=" + strings.Join(opts.nodes.filters, "|"))
	}

	out, err := gd.allocator.ListCandidates(opts.nodes.clusters, opts.nodes.filters, opts.nodes.clusterType, stores)
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

		for o := range out {
			if out[o].EngineID == n.ID {
				if n.Labels == nil {
					n.Labels = make(map[string]string, 5)
				}

				n.Labels[clusterLabel] = out[o].ClusterID
				break
			}
		}

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
	stores := []structs.VolumeRequire{}
	ncpu, memory := 1, 2<<20
	option := scheduleOption{}

	gd.Lock()
	defer gd.Unlock()

	nodes, err := gd.schedule(config, option, stores)
	if err != nil {
		return nil, err
	}

	var (
		count = len(units)
		ready = make([]pendingUnit, 0, count)
		bad   = make([]pendingUnit, 0, count)
		used  = make([]*node.Node, count)
	)

	defer func() {
		// cancel allocation
		if len(bad) > 0 {
			err := gd.allocator.RecycleResource()
			if err != nil {
				// TODO:
			}

			ids := make([]string, len(bad))
			for i := range bad {
				ids[i] = bad[i].swarmID
			}
			gd.cluster.RemovePendingContainer(ids...)
		}
	}()

	for n := range nodes {
		if !selectNodeInDifferentCluster(option.highAvailable, len(units), nodes[n], used) {
			continue
		}

		pu := pendingUnit{
			swarmID:     units[count-1].ID,
			Unit:        units[count-1],
			config:      config.DeepCopy(),
			networkings: make([]string, 0, len(units)),
			volumes:     make([]volume.VolumesCreateBody, 0, len(units)),
		}

		cpuset, err := gd.allocator.AlloctCPUMemory(nodes[n], ncpu, memory, nil)
		if err != nil {
			continue
		}

		volumes, err := gd.allocator.AlloctVolumes(pu.Unit.ID, nodes[n], stores)
		if len(volumes) > 0 {
			pu.volumes = append(pu.volumes, volumes...)
		}
		if err != nil {
			bad = append(bad, pu)
			continue
		}

		id, err := gd.allocator.AlloctNetworking(pu.Unit.ID, "networkingType", 1)
		if len(id) > 0 {
			pu.networkings = append(pu.networkings, id)
		}
		if err != nil {
			bad = append(bad, pu)
			continue
		}

		pu.config.SetSwarmID(pu.swarmID)
		pu.Unit.EngineID = nodes[n].ID

		pu.config.HostConfig.Resources.CpusetCpus = cpuset
		pu.config.HostConfig.Resources.Memory = int64(memory)

		gd.cluster.AddPendingContainer(pu.Name, pu.swarmID, nodes[n].ID, pu.config)

		ready = append(ready, pu)
		used = append(used, nodes[n])

		if count--; count == 0 {
			break
		}
	}

	if count > 0 {
		bad = append(bad, ready...)
	}

	return ready, err
}

func selectNodeInDifferentCluster(highAvailable bool, num int, n *node.Node, used []*node.Node) bool {
	if !highAvailable {
		return true
	}

	if len(used)*2 < num {
		return true
	}

	clusters := make(map[string]int, len(used))
	for i := range used {
		name := used[i].Labels[clusterLabel]
		clusters[name] += 1
	}

	name := n.Labels[clusterLabel]
	sum := clusters[name]
	if sum*2 < num {
		return true
	}

	if len(clusters) > 1 && sum*2 <= num {
		return true
	}

	return false
}