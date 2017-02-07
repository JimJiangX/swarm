package garden

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
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
	ormer        database.Ormer
	kvClient     kvstore.Client
	pluginClient pluginapi.PluginAPI

	allocator allocator
	cluster.Cluster
	scheduler  *scheduler.Scheduler
	TLSConfig  *tls.Config
	authConfig *types.AuthConfig
}

func NewGarden(kvc kvstore.Client, cl cluster.Cluster, scheduler *scheduler.Scheduler, ormer database.Ormer, allocator allocator, tlsConfig *tls.Config) *Garden {
	return &Garden{
		// Mutex:       &scheduler.Mutex,
		kvClient:  kvc,
		allocator: allocator,
		Cluster:   cl,
		ormer:     ormer,
		TLSConfig: tlsConfig,
	}
}

func (gd *Garden) KVClient() kvstore.Client {
	return gd.kvClient
}

func (gd *Garden) AuthConfig() (*types.AuthConfig, error) {
	return gd.ormer.GetAuthConfig()
}

func (gd *Garden) ListServices(ctx context.Context) ([]structs.ServiceSpec, error) {
	// TODO:list services
	return []structs.ServiceSpec{}, nil
}

func (gd *Garden) BuildService(spec structs.ServiceSpec) (*Service, error) {
	if spec.ID == "" {
		spec.ID = utils.Generate32UUID()
	}

	us := make([]database.Unit, spec.Replicas)
	units := make([]structs.UnitSpec, spec.Replicas)

	for i := 0; i < spec.Replicas; i++ {

		uid := utils.Generate32UUID()
		us[i] = database.Unit{
			ID:            uid,
			Name:          fmt.Sprintf("%s_%s", uid[:8], spec.Name), // <unit_id_8bit>_<service_name>
			Type:          "",
			ImageID:       "",
			ImageName:     spec.Image,
			ServiceID:     spec.ID,
			NetworkMode:   "none",
			Status:        0,
			CheckInterval: 10,
			CreatedAt:     time.Now(),
		}

		units[i].Unit = us[i]
	}

	spec.Units = units

	err := gd.ormer.InsertService(spec.Service, us, nil, spec.Users)
	if err != nil {
		return nil, err
	}

	service := gd.NewService(spec)

	service.options = scheduleOption{
		highAvailable: spec.Replicas > 0,
		ContainerSpec: spec.ContainerSpec,
		Options:       spec.Options,
	}
	service.options.nodes.constraint = spec.Constraint

	return service, nil
}

func (gd *Garden) Service(nameOrID string) (*Service, error) {
	svc, err := gd.ormer.GetService(nameOrID)
	if err != nil {
		return nil, err
	}

	spec := structs.ServiceSpec{
		Service: svc,
		// TODO: other params
	}

	s := gd.NewService(spec)

	return s, nil
}

type scheduleOption struct {
	highAvailable bool

	ContainerSpec structs.ContainerSpec

	Options map[string]interface{}

	nodes struct {
		clusterType string
		clusters    []string
		filters     []string
		constraint  []string
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

	list := gd.Cluster.ListEngines(engines...)
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

func (pu pendingUnit) convertToSpec() structs.UnitSpec {
	return structs.UnitSpec{}
}
func (gd *Garden) Allocation(svc *Service) ([]pendingUnit, error) {
	config := cluster.BuildContainerConfig(container.Config{}, container.HostConfig{
		Resources: container.Resources{
			CpusetCpus: svc.options.ContainerSpec.Require.CPU,
			Memory:     svc.options.ContainerSpec.Require.Memory,
		},
	}, network.NetworkingConfig{})

	if len(svc.options.nodes.constraint) > 0 {
		for i := range svc.options.nodes.constraint {
			config.AddConstraint(svc.options.nodes.constraint[i])
		}
	}

	stores := svc.options.ContainerSpec.Volumes

	ncpu, err := strconv.Atoi(config.HostConfig.CpusetCpus)
	if err != nil {
		return nil, err
	}

	gd.Lock()
	defer gd.Unlock()

	nodes, err := gd.schedule(config, svc.options, stores)
	if err != nil {
		return nil, err
	}

	var (
		count = len(svc.spec.Units)
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
			gd.Cluster.RemovePendingContainer(ids...)
		}
	}()

	req, err := svc.Requires(context.Background())
	if err != nil {
		return nil, err
	}

	for n := range nodes {
		units := svc.spec.Units
		if !selectNodeInDifferentCluster(svc.options.highAvailable, len(units), nodes[n], used) {
			continue
		}

		pu := pendingUnit{
			swarmID:     units[count-1].ID,
			Unit:        units[count-1].Unit,
			config:      config.DeepCopy(),
			networkings: make([]string, 0, len(units)),
			volumes:     make([]volume.VolumesCreateBody, 0, len(units)),
		}

		cpuset, err := gd.allocator.AlloctCPUMemory(nodes[n], ncpu, int(config.HostConfig.Memory), nil)
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

		// TODO: networking required
		_ = req

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
		pu.config.HostConfig.Resources.Memory = config.HostConfig.Memory

		gd.Cluster.AddPendingContainer(pu.Name, pu.swarmID, nodes[n].ID, pu.config)

		ready = append(ready, pu)
		used = append(used, nodes[n])

		if count--; count == 0 {
			break
		}
	}

	if count > 0 {
		bad = append(bad, ready...)
	}

	units := make([]structs.UnitSpec, len(ready))
	for i := range ready {
		units[i] = ready[i].convertToSpec()
	}

	svc.spec.Units = units

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
