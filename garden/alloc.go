package garden

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/scheduler"
	"github.com/docker/swarm/scheduler/filter"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/scheduler/strategy"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// add engine labels for schedule
const (
	roomLabel    = "room"
	seatLabel    = "seat"
	nodeLabel    = "node"
	clusterLabel = "Cluster"
)

type allocator interface {
	ListCandidates(clusters, filters []string, stores []structs.VolumeRequire) ([]database.Node, error)

	AlloctCPUMemory(config *cluster.ContainerConfig, node *node.Node, ncpu, memory int64, reserved []string) (string, error)

	AlloctVolumes(config *cluster.ContainerConfig, id string, n *node.Node, stores []structs.VolumeRequire) ([]volume.VolumesCreateBody, error)

	AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string, requires []structs.NetDeviceRequire) ([]database.IP, error)

	RecycleResource() error
}

func (gd *Garden) Service(nameOrID string) (*Service, error) {
	info, err := gd.ormer.GetServiceInfo(nameOrID)
	if err != nil {
		return nil, err
	}

	spec := structs.ServiceSpec{
		Service: structs.Service(info.Service),
		// TODO: other params
	}

	s := gd.NewService(spec)

	return s, nil
}

// TODO:convert to structs.UnitSpec
func convertUnitInfoToSpec(info database.UnitInfo) structs.UnitSpec {
	return structs.UnitSpec{
		Unit: structs.Unit(info.Unit),

		//	Require, Limit struct {
		//		CPU    string
		//		Memory int64
		//	}

		Engine: struct {
			ID   string
			Addr string
		}{
			ID:   info.Engine.EngineID,
			Addr: info.Engine.Addr,
		},

		//	Networking struct {
		//		Type    string
		//		Devices string
		//		Mask    int
		//		IPs     []struct {
		//			Name  string
		//			IP    string
		//			Proto string
		//		}
		//		Ports []struct {
		//			Name string
		//			Port int
		//		}
		//	}

		//	Volumes []struct {
		//		Type    string
		//		Driver  string
		//		Size    int
		//		Options map[string]interface{}
		//	}
	}
}

func (gd *Garden) ListServices(ctx context.Context) ([]structs.ServiceSpec, error) {
	list, err := gd.ormer.ListServicesInfo()
	if err != nil {
		return nil, err
	}

	services := make([]structs.ServiceSpec, len(list))
	for i := range list {
		units := make([]structs.UnitSpec, 0, len(list[i].Units))

		for u := range list[i].Units {
			units = append(units, convertUnitInfoToSpec(list[i].Units[u]))
		}

		services = append(services, structs.ServiceSpec{
			Replicas: len(list[i].Units),
			Service:  structs.Service(list[i].Service),
			//	Users:    list[i].Users,
			Units: units,
		})
	}

	return services, nil
}

func (gd *Garden) validServiceSpec(spec structs.ServiceSpec) error {
	return nil
}

func (gd *Garden) BuildService(spec structs.ServiceSpec) (*Service, *database.Task, error) {
	err := gd.validServiceSpec(spec)
	if err != nil {
		return nil, nil, err
	}

	//	image, err := gd.ormer.GetImage(spec.Image)
	//	if err != nil {
	//		return nil, err
	//	}

	if spec.ID == "" {
		spec.ID = utils.Generate32UUID()
	}

	desc := bytes.NewBuffer(nil)
	err = json.NewEncoder(desc).Encode(spec)
	if err != nil {
		return nil, nil, err
	}

	spec.Desc = desc.String()

	us := make([]database.Unit, spec.Replicas)
	units := make([]structs.UnitSpec, spec.Replicas)

	for i := 0; i < spec.Replicas; i++ {

		uid := utils.Generate32UUID()
		us[i] = database.Unit{
			ID:          uid,
			Name:        fmt.Sprintf("%s_%s", spec.Name, uid[:8]), // <service_name>_<unit_id_8bit>
			Type:        "",
			ServiceID:   spec.ID,
			NetworkMode: "none",
			Status:      0,
			CreatedAt:   time.Now(),
		}

		units[i].Unit = structs.Unit(us[i])
	}

	spec.Units = units

	t := database.NewTask(spec.Name, database.ServiceCreateTask, spec.ID, spec.Desc, "", 300)

	err = gd.ormer.InsertService(database.Service(spec.Service), us, &t, nil)
	if err != nil {
		return nil, nil, err
	}

	service := gd.NewService(spec)

	service.options = scheduleOption{
		highAvailable: spec.Replicas > 0,
		require:       spec.Require,
		options:       spec.Options,
	}
	service.options.nodes.clusters = spec.Clusters
	service.options.nodes.constraints = spec.Constraints

	return service, &t, nil
}

type scheduleOption struct {
	highAvailable bool

	require structs.UnitRequire

	options map[string]interface{}

	nodes struct {
		// clusterType string
		clusters    []string
		filters     []string
		constraints []string
	}

	scheduler struct {
		strategy string
		filters  []string
	}
}

func (gd *Garden) schedule(ctx context.Context, config *cluster.ContainerConfig, opts scheduleOption) ([]*node.Node, error) {
	_scheduler := gd.scheduler

	if opts.scheduler.strategy != "" && len(opts.scheduler.filters) > 0 {
		strategy, _ := strategy.New(opts.scheduler.strategy)
		filters, _ := filter.New(opts.scheduler.filters)

		if strategy != nil && len(filters) > 0 {
			_scheduler = scheduler.New(strategy, filters)
		}
	}

	select {
	default:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	out, err := gd.allocator.ListCandidates(opts.nodes.clusters, opts.nodes.filters, opts.require.Volumes)
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
				n.Labels[nodeLabel] = out[o].ID
				n.Labels[roomLabel] = out[o].Room
				n.Labels[seatLabel] = out[o].Seat
				break
			}
		}

		nodes = append(nodes, n)
	}

	select {
	default:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	nodes, err = _scheduler.SelectNodesForContainer(nodes, config)

	return nodes, err
}

type pendingUnit struct {
	database.Unit
	swarmID string

	config      *cluster.ContainerConfig
	networkings []database.IP
	volumes     []volume.VolumesCreateBody
}

func (pu pendingUnit) convertToSpec() structs.UnitSpec {
	return structs.UnitSpec{}
}

func (gd *Garden) Allocation(ctx context.Context, svc *Service) ([]pendingUnit, error) {
	opts := svc.options
	config := cluster.BuildContainerConfig(container.Config{}, container.HostConfig{
		Resources: container.Resources{
			CpusetCpus: strconv.Itoa(opts.require.Require.CPU),
			Memory:     opts.require.Require.Memory,
		},
	}, network.NetworkingConfig{})

	for i := range opts.nodes.constraints {
		config.AddConstraint(opts.nodes.constraints[i])
	}
	if len(opts.nodes.filters) > 0 {
		config.AddConstraint(nodeLabel + "!=" + strings.Join(opts.nodes.filters, "|"))
	}
	if len(opts.nodes.clusters) > 0 {
		config.AddConstraint(clusterLabel + "==" + strings.Join(opts.nodes.clusters, "|"))
	}

	gd.Lock()
	defer gd.Unlock()

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	nodes, err := gd.schedule(ctx, config, opts)
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
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
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

	select {
	default:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for n := range nodes {
		units := svc.spec.Units
		if !selectNodeInDifferentCluster(opts.highAvailable, len(units), nodes[n], used) {
			continue
		}

		pu := pendingUnit{
			swarmID:     units[count-1].ID,
			Unit:        database.Unit(units[count-1].Unit),
			config:      config.DeepCopy(),
			networkings: make([]database.IP, 0, len(units)),
			volumes:     make([]volume.VolumesCreateBody, 0, len(units)),
		}

		cpuset, err := gd.allocator.AlloctCPUMemory(pu.config, nodes[n], int64(opts.require.Require.CPU), config.HostConfig.Memory, nil)
		if err != nil {
			continue
		}

		volumes, err := gd.allocator.AlloctVolumes(pu.config, pu.Unit.ID, nodes[n], opts.require.Volumes)
		if len(volumes) > 0 {
			pu.volumes = append(pu.volumes, volumes...)
		}
		if err != nil {
			bad = append(bad, pu)
			continue
		}

		networkings, err := gd.allocator.AlloctNetworking(pu.config, nodes[n].ID, pu.Unit.ID, opts.require.Networkings)
		if len(networkings) > 0 {
			pu.networkings = append(pu.networkings, networkings...)
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
		clusters[name]++
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
