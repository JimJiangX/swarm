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
	roomLabel             = "room"
	seatLabel             = "seat"
	nodeLabel             = "nodeID"
	engineLabel           = "node"
	clusterLabel          = "cluster"
	serviceTagLabel       = "service.tag"
	sanLabel              = "SAN_ID"
	maxContainerLable     = "containerslots" // scheduler/filter/slots.go
	networkPartitionLable = "network_partition"
)

func getImage(orm database.SysConfigOrmer, version string) (string, error) {
	reg, err := orm.GetRegistry()
	if err != nil {
		return "", err
	}

	name := fmt.Sprintf("%s:%d/%s", reg.Domain, reg.Port, version)

	return name, nil
}

// BuildService build a pointer of Service,
// 根据 ServiceSpec 生成 Service、scheduleOption、[]unit、Task，并记录到数据库。
func (gd *Garden) BuildService(spec structs.ServiceSpec) (*Service, *database.Task, error) {
	options := newScheduleOption(spec)

	im, err := gd.ormer.GetImageVersion(spec.Image.ID)
	if err != nil {
		return nil, nil, err
	}
	spec.Image = structs.ImageVersion{
		ID:    im.ID,
		Name:  im.Name,
		Major: im.Major,
		Minor: im.Minor,
		Patch: im.Patch,
		Dev:   im.Dev,
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

	out, err := actor.ListCandidates(opts.Nodes.Clusters, opts.Nodes.Filters, opts.Nodes.Networkings, opts.Require.Volumes)
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, errors.New("no one node that satisfies")
	}

	nodes, err := setNodesWithLable(out, gd.Cluster, gd.Ormer())
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, errors.New("no one node that satisfies")
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

func setNodesWithLable(out []database.Node, cl cluster.Cluster, iface database.ClusterIface) ([]*node.Node, error) {
	clusters, err := iface.ListClusters()
	if err != nil {
		return nil, err
	}
	if len(clusters) == 0 {
		return nil, errors.New("List Cluster result empty")
	}

	clusterMap := make(map[string]string, len(clusters))
	for i := range clusters {
		clusterMap[clusters[i].ID] = clusters[i].NetworkPartition
	}

	nodeMap := make(map[string]database.Node, len(out))
	ids := make([]string, 0, len(out))
	for i := range out {
		if out[i].EngineID == "" {
			continue
		}

		nodeMap[out[i].EngineID] = out[i]
		ids = append(ids, out[i].EngineID)
	}

	engines := cl.ListEngines(ids...)
	nodes := make([]*node.Node, 0, len(engines))

	for i := range engines {
		n := node.NewNode(engines[i])
		nt, ok := nodeMap[n.ID]
		if !ok {
			continue
		}

		if n.Labels == nil {
			n.Labels = make(map[string]string, 5)
		}
		if nt.Storage != "" {
			n.Labels[sanLabel] = nt.Storage
		}

		n.Labels[clusterLabel] = nt.ClusterID
		n.Labels[nodeLabel] = nt.ID
		n.Labels[roomLabel] = nt.Room
		n.Labels[seatLabel] = nt.Seat
		n.Labels[networkPartitionLable] = clusterMap[nt.ClusterID]
		//	n.Labels[maxContainerLable] = strconv.Itoa(nt.MaxContainer)

		nodes = append(nodes, n)
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
	sl := tasklock.NewServiceTask(svc.ID(), svc.so, nil,
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

func buildServiceContainerConfig(svc *Service) (*cluster.ContainerConfig, error) {
	version, err := getImage(svc.so, svc.spec.Image.Image())
	if err != nil {
		return nil, err
	}

	config := cluster.BuildContainerConfig(container.Config{
		Tty:       true,
		OpenStdin: true,
		Image:     version,
	}, container.HostConfig{
		NetworkMode: "none",
		Binds:       []string{"/etc/localtime:/etc/localtime:ro"},
		Resources: container.Resources{
			CpusetCpus: strconv.Itoa(svc.options.Require.Require.CPU),
			Memory:     svc.options.Require.Require.Memory,
		},
	}, network.NetworkingConfig{})

	config.Config.Labels["mgm.unit.type"] = svc.spec.Image.Name
	config.Config.Labels["mgm.image.id"] = svc.spec.Image.ID
	config.Config.Labels[serviceTagLabel] = svc.svc.Tag

	addContainerConfigConstraint(config, svc.options)

	return config, nil
}

func addContainerConfigConstraint(config *cluster.ContainerConfig, opts scheduleOption) {
	for i := range opts.Nodes.Constraints {
		if opts.Nodes.Constraints[i] != "" {
			config.AddConstraint(opts.Nodes.Constraints[i])
		}
	}

	if len(opts.Nodes.Filters) > 0 {
		tmp := make([]string, 0, len(opts.Nodes.Filters))
		for i := range opts.Nodes.Filters {
			if opts.Nodes.Filters[i] != "" {
				tmp = append(tmp, opts.Nodes.Filters[i])
			}
		}

		config.AddConstraint(nodeLabel + "!=" + strings.Join(tmp, "|"))
	}

	if len(opts.Nodes.Clusters) > 0 {
		tmp := make([]string, 0, len(opts.Nodes.Clusters))
		for i := range opts.Nodes.Clusters {
			if opts.Nodes.Clusters[i] != "" {
				tmp = append(tmp, opts.Nodes.Clusters[i])
			}
		}

		config.AddConstraint(clusterLabel + "==" + strings.Join(tmp, "|"))
	}
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

func sortByLabel(nodes []*node.Node, labelName string) map[string][]*node.Node {
	if len(nodes) == 0 {
		return nil
	}

	type set struct {
		label string
		nodes []*node.Node
	}

	sets := make([]set, 0, 10)

loop:
	for i := range nodes {
		if nodes[i] == nil {
			continue
		}

		label, ok := nodes[i].Labels[labelName]
		if !ok || label == "" {
			continue
		}

		for k := range sets {
			if sets[k].label == label {
				sets[k].nodes = append(sets[k].nodes, nodes[i])

				continue loop
			}
		}

		// label not exist in sets,so append it
		list := make([]*node.Node, 1, len(nodes)-len(nodes)/2)
		list[0] = nodes[i]

		sets = append(sets, set{
			label: label,
			nodes: list,
		})
	}

	out := make(map[string][]*node.Node, len(sets))

	for i := range sets {
		out[sets[i].label] = sets[i].nodes
	}

	return out
}

func filterNodeByStorage(n *node.Node) bool {
	name, ok := n.Labels[sanLabel]
	if !ok || name == "" {
		return false
	}

	return true
}

func selectNodeInDiffStorage(highAvailable bool, num int, n *node.Node, used []*node.Node) bool {
	name, ok := n.Labels[sanLabel]
	if !ok || name == "" {
		return false
	}

	if !highAvailable {
		return true
	}

	if len(used)*2 < num {
		return true
	}

	sans := make(map[string]int, len(used))
	for i := range used {
		name := used[i].Labels[sanLabel]
		sans[name] = sans[name] + 1
	}

	sum := sans[name]
	if sum*2 < num {
		return true
	}

	if len(sans) > 1 && sum*2 <= num {
		return true
	}

	return false
}

func selectNodeInDiffNetworkPartition(highAvailable bool, num int, n *node.Node, used []*node.Node) bool {
	if !highAvailable {
		return true
	}

	if len(used)*2 < num {
		return true
	}

	partitions := make(map[string]int, len(used))
	for i := range used {
		name := used[i].Labels[networkPartitionLable]
		partitions[name] = partitions[name] + 1
	}

	name, ok := n.Labels[networkPartitionLable]
	if !ok {
		return false
	}

	sum := partitions[name]
	if sum*2 < num {
		return true
	}

	if len(partitions) > 1 && sum*2 <= num {
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

func isNodeAttempted(n *node.Node, attempt []*node.Node) bool {
	if n == nil || n.ID == "" {
		return true
	}

	for i := range attempt {
		if n.ID == attempt[i].ID {
			return true
		}
	}

	return false
}

func isNodeMatch(isSAN, highAvailable bool, num int, n *node.Node, used []*node.Node) bool {

	if highAvailable &&
		!selectNodeInDiffNetworkPartition(highAvailable, num, n, used) {
		return false
	}

	if isSAN && !filterNodeByStorage(n) {
		return false
	}

	if isSAN && highAvailable &&
		!selectNodeInDiffStorage(highAvailable, num, n, used) {
		return false
	}

	return true
}

type nodeSelectStrategy struct {
	isSAN         bool
	highAvailable bool
	num           int
	nodes         []*node.Node
	attemptNodes  []*node.Node
	category      map[string][]*node.Node
}

func newNodeSelectStrategy(isSAN, highAvailable bool, num int, nodes []*node.Node) *nodeSelectStrategy {
	ns := &nodeSelectStrategy{
		isSAN:         isSAN,
		highAvailable: highAvailable,
		num:           num,
		nodes:         nodes,
		attemptNodes:  make([]*node.Node, 0, len(nodes)),
	}

	if isSAN && highAvailable {
		// cache sort by storage
		ns.category = sortByLabel(nodes, sanLabel)
	}

	return ns
}

func (cs *nodeSelectStrategy) selectNode(used []*node.Node) *node.Node {
	if cs.isSAN && cs.highAvailable &&
		len(cs.category) > 1 && len(used) < len(cs.category) {
		// 均匀分配至各个SAN集群中
		sans := make(map[string]int, len(used))
		for i := range used {
			name := used[i].Labels[sanLabel]
			sans[name] = sans[name] + 1
		}

		for label, nodes := range cs.category {
			if sans[label] > 0 {
				continue
			}

			for i := range nodes {

				if isNodeAttempted(nodes[i], cs.attemptNodes) {
					continue
				}

				if !isNodeMatch(cs.isSAN, cs.highAvailable, cs.num, nodes[i], used) {
					continue
				}

				cs.attemptNodes = append(cs.attemptNodes, nodes[i])

				return nodes[i]
			}
		}
	}

	for i := range cs.nodes {
		if isNodeAttempted(cs.nodes[i], cs.attemptNodes) {
			continue
		}

		if !isNodeMatch(cs.isSAN, cs.highAvailable, cs.num, cs.nodes[i], used) {
			continue
		}

		cs.attemptNodes = append(cs.attemptNodes, cs.nodes[i])

		return cs.nodes[i]
	}

	return nil
}

func (gd *Garden) allocation(ctx context.Context, actor alloc.Allocator, svc *Service,
	units []database.Unit, vr, nr bool) (ready []pendingUnit, err error) {

	config, err := buildServiceContainerConfig(svc)
	if err != nil {
		return nil, err
	}

	opts := svc.options

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
		units, err = svc.so.ListUnitByServiceID(svc.ID())
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
		field = logrus.WithField("Service", svc.Name())
	)

	recycle := func() error {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		// cancel allocation
		if len(bad) == 0 {
			return nil
		}

		field.Infof("allocation recycle volume resource:%d", len(bad))

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

		return err
	}

	defer recycle()

	count := replicas
	used := make([]pendingUnit, 0, count)
	usedNodes := make([]*node.Node, 0, count)

	ns := newNodeSelectStrategy(isSANStorage(opts.Require.Volumes),
		opts.HighAvailable, replicas, candidates)

	for {

		select {
		default:
		case <-ctx.Done():
			err = errors.Wrapf(ctx.Err(), "%+v\n", err)
			return nil, err
		}

		candidate := ns.selectNode(usedNodes)
		if candidate == nil {
			break
		}

		if err = recycle(); err != nil {
			field.Errorf("recycle resource failed,%+v", err)
		}

		pu, err := pendingAlloc(actor, units[count-1], candidate, opts, config, vr, nr)
		if err != nil {
			bad = append(bad, pu)

			field.Errorf("pending alloc:node=%s,%+v", candidate.Name, err)

			continue
		}

		err = gd.Cluster.AddPendingContainer(pu.Name, pu.swarmID, candidate.ID, pu.config)
		if err != nil {
			bad = append(bad, pu)
			field.Debugf("AddPendingContainer:node=%s,%+v", candidate.Name, err)
			continue
		}

		usedNodes = append(usedNodes, candidate)
		used = append(used, pu)
		if count--; count == 0 {
			return used, nil
		}
	}

	bad = append(bad, used...)

	return nil, errors.Errorf("not enough nodes for allocation,%d units waiting", replicas)
}

func (gd *Garden) allocationV2(ctx context.Context, actor alloc.Allocator, svc *Service,
	units []database.Unit, vr, nr bool) (ready []pendingUnit, err error) {

	config, err := buildServiceContainerConfig(svc)
	if err != nil {
		return nil, err
	}

	opts := svc.options

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
		units, err = svc.so.ListUnitByServiceID(svc.ID())
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
		field = logrus.WithField("Service", svc.Name())
	)

	defer func() error {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		// cancel allocation
		if len(bad) == 0 {
			return nil
		}

		field.Infof("allocation recycle volume resource:%d", len(bad))

		ids := make([]string, len(bad))
		for i := range bad {
			ids[i] = bad[i].swarmID
		}
		gd.Cluster.RemovePendingContainer(ids...)

		return err

		//		ips := make([]database.IP, 0, len(bad))
		//		lvs := make([]database.Volume, 0, len(bad)*2)
		//		for i := range bad {
		//			ips = append(ips, bad[i].networkings...)
		//			lvs = append(lvs, bad[i].volumes...)
		//		}
		//		_err := actor.RecycleResource(ips, lvs)
		//		if _err != nil {
		//			err = fmt.Errorf("%+v\nRecycle resources error:%+v", err, _err)
		//		}

		//		return _err
	}()

	count := replicas
	used := make([]pendingUnit, 0, count)
	usedNodes := make([]*node.Node, 0, count)
	isSAN := isSANStorage(opts.Require.Volumes)

	for i := range candidates {
		if !isNodeMatch(isSAN, opts.HighAvailable, replicas, candidates[i], usedNodes) {
			continue
		}

		pu, err := pendingAlloc(actor, units[count-1], candidates[i], opts, config, vr, nr)
		if err != nil {
			bad = append(bad, pu)
			bad = append(bad, used...)

			field.Errorf("pending alloc:node=%s,%+v", candidates[i].Name, err)

			return nil, err
		}

		err = gd.Cluster.AddPendingContainer(pu.Name, pu.swarmID, candidates[i].ID, pu.config)
		if err != nil {
			bad = append(bad, pu)
			field.Debugf("AddPendingContainer:node=%s,%+v", candidates[i].Name, err)
			continue
		}

		used = append(used, pu)
		usedNodes = append(usedNodes, candidates[i])

		if count--; count == 0 {
			return used, nil
		}
	}

	bad = append(bad, used...)

	return nil, errors.Errorf("not enough nodes for allocation,%d units waiting", replicas)
}

// TODO:remove
func (gd *Garden) allocationV3(ctx context.Context, actor alloc.Allocator, svc *Service,
	units []database.Unit, vr, nr bool) (ready []pendingUnit, err error) {

	config, err := buildServiceContainerConfig(svc)
	if err != nil {
		return nil, err
	}

	opts := svc.options

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
		units, err = svc.so.ListUnitByServiceID(svc.ID())
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
		field = logrus.WithField("Service", svc.Name())
	)

	recycle := func() error {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		// cancel allocation
		if len(bad) == 0 {
			return err
		}

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

		return err
	}

	defer recycle()

	out := sortByLabel(candidates, clusterLabel)
	count := replicas
	usedNodes := make([]*node.Node, 0, count)
	isSAN := isSANStorage(opts.Require.Volumes)

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

		used := make([]pendingUnit, 0, count)

		for i := range nodes {
			if !isNodeMatch(isSAN, opts.HighAvailable, replicas, candidates[i], usedNodes) {
				continue
			}

			pu, err := pendingAlloc(actor, units[count-1], nodes[i], opts, config, vr, nr)
			if err != nil {
				bad = append(bad, pu)
				field.Debugf("pending alloc:node=%s,%+v", nodes[i].Name, err)
				continue
			}

			err = gd.Cluster.AddPendingContainer(pu.Name, pu.swarmID, nodes[i].ID, pu.config)
			if err != nil {
				bad = append(bad, pu)
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
