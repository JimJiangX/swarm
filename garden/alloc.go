package garden

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/scheduler"
	"github.com/docker/swarm/scheduler/filter"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/scheduler/strategy"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const containerKV = "/containers/"

// add engine labels for schedule
const (
	roomLabel       = "room"
	seatLabel       = "seat"
	nodeLabel       = "nodeID"
	engineLabel     = "node"
	clusterLabel    = "cluster"
	serviceTagLabel = "service.tag"
	sanLabel        = "SAN_ID"
)

func getImage(orm database.ImageOrmer, version string) (database.Image, string, error) {
	im, err := orm.GetImageVersion(version)
	if err != nil {
		return im, "", err
	}

	reg, err := orm.GetRegistry()
	if err != nil {
		return im, "", err
	}

	name := fmt.Sprintf("%s:%d/%s", reg.Domain, reg.Port, im.Version())

	return im, name, nil
}

// BuildService build a pointer of Service,
// 根据 ServiceSpec 生成 Service、scheduleOption、[]unit、Task，并记录到数据库。
func (gd *Garden) BuildService(spec structs.ServiceSpec) (*Service, *database.Task, error) {
	options := newScheduleOption(spec)

	im, err := gd.ormer.GetImageVersion(spec.Image)
	if err != nil {
		return nil, nil, err
	}
	if spec.ID == "" {
		spec.ID = utils.Generate32UUID()
	}
	svc, err := convertStructsService(spec, options)
	if err != nil {
		return nil, nil, err
	}
	svc.Desc.ImageID = im.ID
	svc.Status = statusServcieBuilding

	us := make([]database.Unit, spec.Arch.Replicas)
	units := make([]structs.UnitSpec, spec.Arch.Replicas)

	for i := range us {
		uid := utils.Generate32UUID()
		us[i] = database.Unit{
			ID:          uid,
			Name:        fmt.Sprintf("%s_%s", uid[:8], spec.Tag), // <unit_id_8bit>_<service_tag>
			Type:        im.Name,
			ServiceID:   spec.ID,
			NetworkMode: "none",
			Status:      0,
			CreatedAt:   time.Now(),
		}

		units[i].Unit = structs.Unit(us[i])
	}

	spec.Units = units

	t := database.NewTask(spec.Name, database.ServiceRunTask, spec.ID, svc.Desc.ID, nil, 300)

	err = gd.ormer.InsertService(svc, us, &t)
	if err != nil {
		return nil, nil, err
	}

	service := gd.NewService(&spec, &svc)

	service.options = options

	return service, &t, nil
}

type scheduleOption struct {
	HighAvailable bool `json:"HighAvailable,omitempty"`

	Require structs.UnitRequire `json:"UnitRequire,omitempty"`

	Nodes struct {
		Networkings map[string][]string `json:"Networkings,omitempty"` // key:cluster id,value:networking id slice
		Clusters    []string            `json:"Clusters,omitempty"`
		Filters     []string            `json:"Filters,omitempty"`
		Constraints []string            `json:"Constraints,omitempty"`
	} `json:"Nodes,omitempty"`

	Scheduler struct {
		Strategy string   `json:"Strategy,omitempty"`
		Filters  []string `json:"Filters,omitempty"`
	} `json:"Scheduler,omitempty"`
}

func newScheduleOption(spec structs.ServiceSpec) scheduleOption {
	opts := scheduleOption{
		HighAvailable: spec.HighAvailable,
		Require:       *spec.Require,
	}

	opts.Nodes.Constraints = spec.Constraints
	opts.Nodes.Networkings = spec.Networkings
	opts.Nodes.Clusters = spec.Clusters

	return opts
}

func (gd *Garden) schedule(ctx context.Context, actor alloc.Allocator, config *cluster.ContainerConfig, opts scheduleOption) ([]*node.Node, error) {
	_scheduler := gd.scheduler

	if opts.Scheduler.Strategy != "" && len(opts.Scheduler.Filters) > 0 {
		strategy, _ := strategy.New(opts.Scheduler.Strategy)
		filters, _ := filter.New(opts.Scheduler.Filters)

		if strategy != nil && len(filters) > 0 {
			_scheduler = scheduler.New(strategy, filters)
		}
	}

	select {
	default:
	case <-ctx.Done():
		return nil, errors.WithStack(ctx.Err())
	}

	out, err := actor.ListCandidates(opts.Nodes.Clusters, opts.Nodes.Filters, opts.Require.Volumes)
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, errors.New("no one node that satisfies")
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
				n.Labels[sanLabel] = out[o].Storage
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
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return nodes, nil
}

type pendingUnit struct {
	database.Unit
	swarmID string

	config      *cluster.ContainerConfig
	networkings []database.IP
	volumes     []database.Volume
}

// Allocation alloc resources for building containers on hosts
func (gd *Garden) Allocation(ctx context.Context, actor alloc.Allocator, svc *Service) (ready []pendingUnit, err error) {
	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceAllocating, statusServiceAllocated, statusServiceAllocateFailed)

	err = sl.Run(
		func(val int) bool {
			return val == statusServcieBuilding
		},
		func() error {
			ready, err = gd.allocation(ctx, actor, svc, nil, true, true)
			return err
		},
		false)

	return
}

func (gd *Garden) allocation(ctx context.Context, actor alloc.Allocator, svc *Service,
	units []database.Unit, vr, nr bool) (ready []pendingUnit, err error) {

	im, version, err := getImage(gd.Ormer(), svc.svc.Desc.Image)
	if err != nil {
		return nil, err
	}

	opts := svc.options
	isSAN := isSANStorage(opts.Require.Volumes)

	config := cluster.BuildContainerConfig(container.Config{
		Tty:       true,
		OpenStdin: true,
		Image:     version,
	}, container.HostConfig{
		NetworkMode: "none",
		Binds:       []string{"/etc/localtime:/etc/localtime:ro"},
		Resources: container.Resources{
			CpusetCpus: strconv.Itoa(opts.Require.Require.CPU),
			Memory:     opts.Require.Require.Memory,
		},
	}, network.NetworkingConfig{})

	config.Config.Labels["mgm.unit.type"] = im.Name
	config.Config.Labels[serviceTagLabel] = svc.svc.Tag

	{
		for i := range opts.Nodes.Constraints {
			config.AddConstraint(opts.Nodes.Constraints[i])
		}
		if len(opts.Nodes.Filters) > 0 {
			config.AddConstraint(nodeLabel + "!=" + strings.Join(opts.Nodes.Filters, "|"))
		}
		if out := opts.Nodes.Clusters; len(out) > 0 {
			config.AddConstraint(clusterLabel + "==" + strings.Join(out, "|"))
		}
		if isSAN {
			config.AddConstraint(sanLabel + `!=""`)
		}
	}

	gd.Lock()
	defer gd.Unlock()

	gd.scheduler.Lock()
	candidates, err := gd.schedule(ctx, actor, config, opts)
	if err != nil {
		gd.scheduler.Unlock()
		return nil, err
	}
	gd.scheduler.Unlock()

	if units == nil {
		units, err = svc.so.ListUnitByServiceID(svc.svc.ID)
		if err != nil {
			return nil, err
		}
	}

	replicas := len(units)

	if len(candidates) < replicas {
		return nil, errors.Errorf("not enough nodes for allocation,%d<%d", len(candidates), replicas)
	}

	var (
		bad   = make([]pendingUnit, 0, replicas)
		field = logrus.WithField("Service", svc.svc.Name)
	)

	recycle := func() error {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		// cancel allocation
		if len(bad) > 0 {
			ids := make([]string, len(bad))
			for i := range bad {
				ids[i] = bad[i].swarmID
			}
			gd.Cluster.RemovePendingContainer(ids...)

			ips := make([]database.IP, 0, len(bad))
			lvs := make([]database.Volume, 0, len(bad)*2)
			for i := range bad {
				ips = append(ips, bad[i].networkings...)
				lvs = append(lvs, bad[i].volumes...)
			}
			_err := actor.RecycleResource(ips, lvs)
			if _err != nil {
				err = fmt.Errorf("%+v\nRecycle resources error:%+v", err, _err)
			} else {
				bad = make([]pendingUnit, 0, replicas)
			}

			return _err
		}

		return nil
	}

	defer recycle()

	out := sortByCluster(candidates, opts.Nodes.Clusters)

	for _, nodes := range out {
		select {
		default:
		case <-ctx.Done():
			return nil, errors.WithStack(ctx.Err())
		}

		err := recycle()
		if err != nil {
			field.Debugf("Recycle resources error:%+v", err)
		}

		count := replicas
		used := make([]pendingUnit, 0, count)
		usedNodes := make([]*node.Node, 0, count)

		if len(nodes) < count {
			continue
		}

		for i := range nodes {
			if isSAN && opts.HighAvailable {
				if !selectNodeInDifferentStorage(opts.HighAvailable, replicas, nodes[i], usedNodes) {
					continue
				}
			}

			pu, err := pendingAlloc(actor, units[count-1], nodes[i], opts, config, vr, nr)
			if err != nil {
				bad = append(bad, pu)
				field.Debugf("pending alloc:node=%s,%+v", nodes[i].Name, err)
				continue
			}

			err = gd.Cluster.AddPendingContainer(pu.Name, pu.swarmID, nodes[i].ID, pu.config)
			if err != nil {
				field.Debugf("AddPendingContainer:node=%s,%+v", nodes[i].Name, err)
				continue
			}

			used = append(used, pu)
			usedNodes = append(usedNodes, nodes[i])

			if count--; count == 0 {
				ready = used

				return ready, nil
			}
		}
		if count > 0 {
			bad = append(bad, used...)
		}
	}

	return nil, errors.Errorf("not enough nodes for allocation,%d units waiting", replicas)
}

func pendingAlloc(actor alloc.Allocator, unit database.Unit,
	node *node.Node, opts scheduleOption,
	config *cluster.ContainerConfig, vr, nr bool) (pendingUnit, error) {
	pu := pendingUnit{
		swarmID:     unit.ID,
		Unit:        unit,
		config:      config.DeepCopy(),
		networkings: make([]database.IP, 0, 2),
		volumes:     make([]database.Volume, 0, 3),
	}

	_, err := actor.AlloctCPUMemory(pu.config, node, int64(opts.Require.Require.CPU), config.HostConfig.Memory, nil)
	if err != nil {
		logrus.Debugf("AlloctCPUMemory:node=%s,%s", node.Name, err)
		return pu, err
	}

	if nr {
		netlist := opts.Nodes.Networkings[node.Labels[clusterLabel]]

		networkings, err := actor.AlloctNetworking(pu.config, node.ID, pu.Unit.ID, netlist, opts.Require.Networks)
		if len(networkings) > 0 {
			pu.networkings = append(pu.networkings, networkings...)
		}
		if err != nil {
			logrus.Debugf("AlloctNetworking:node=%s,%s", node.Name, err)
			return pu, err
		}
	}

	if vr {
		lvs, err := actor.AlloctVolumes(pu.config, pu.Unit.ID, node, opts.Require.Volumes)
		if len(lvs) > 0 {
			pu.volumes = append(pu.volumes, lvs...)
		}
		if err != nil {
			logrus.Debugf("AlloctVolumes:node=%s,%s", node.Name, err)
			return pu, err
		}
	}

	pu.config.SetSwarmID(pu.swarmID)
	pu.Unit.EngineID = node.ID
	pu.config.Config.Env = append(pu.config.Config.Env, "C_NAME="+pu.Unit.Name)
	pu.config.Config.Labels["mgm.unit.id"] = pu.Unit.ID

	return pu, err
}

func sortByCluster(nodes []*node.Node, clusters []string) [][]*node.Node {
	if len(nodes) == 0 {
		return nil
	}

	type set struct {
		cluster string
		nodes   []*node.Node
	}

	sets := make([]set, 0, len(clusters))

loop:
	for i := range nodes {
		if nodes[i] == nil {
			continue
		}

		label := nodes[i].Labels[clusterLabel]

		if len(clusters) > 0 {
			exist := false
		cluster:
			for c := range clusters {
				if clusters[c] == label {
					exist = true
					break cluster
				}
			}
			if !exist {
				continue loop
			}
		}

		for k := range sets {
			if sets[k].cluster == label {
				sets[k].nodes = append(sets[k].nodes, nodes[i])

				continue loop
			}
		}

		// label not exist in sets,so append it
		list := make([]*node.Node, 1, len(nodes)-len(nodes)/2)
		list[0] = nodes[i]

		sets = append(sets, set{
			cluster: label,
			nodes:   list,
		})
	}

	out := make([][]*node.Node, len(sets))

	for i := range sets {
		out[i] = sets[i].nodes
	}

	return out
}

func selectNodeInDifferentStorage(highAvailable bool, num int, n *node.Node, used []*node.Node) bool {
	if !highAvailable {
		return true
	}

	if len(used)*2 < num {
		return true
	}

	clusters := make(map[string]int, len(used))
	for i := range used {
		name := used[i].Labels[sanLabel]
		clusters[name]++
	}

	name := n.Labels[sanLabel]
	sum := clusters[name]
	if sum*2 < num {
		return true
	}

	if len(clusters) > 1 && sum*2 <= num {
		return true
	}

	return false
}

func isSANStorage(vrs []structs.VolumeRequire) bool {
	for i := range vrs {
		if vrs[i].Type == storage.SANStore {
			return true
		}
	}

	return false
}
