package garden

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	stderr "errors"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
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

// add engine labels for schedule
const (
	roomLabel    = "room"
	seatLabel    = "seat"
	nodeLabel    = "node"
	clusterLabel = "cluster"
)

var (
	errConvertServiceSpec = stderr.New("convert structs.ServiceSpec to database.Service")
)

type allocator interface {
	ListCandidates(clusters, filters []string, stores []structs.VolumeRequire) ([]database.Node, error)

	AlloctCPUMemory(config *cluster.ContainerConfig, node *node.Node, ncpu, memory int64, reserved []string) (string, error)

	AlloctVolumes(config *cluster.ContainerConfig, id string, n *node.Node, stores []structs.VolumeRequire) ([]volume.VolumesCreateBody, error)

	AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string, networkings []string, requires []structs.NetDeviceRequire) ([]database.IP, error)

	RecycleResource() error
}

func convertService(svc database.Service) structs.Service {
	if svc.Desc == nil {
		svc.Desc = &database.ServiceDesc{}
	}

	return structs.Service{
		ID:                   svc.ID,
		Name:                 svc.Name,
		Image:                svc.Desc.Image,
		Desc:                 svc.DescID,
		Architecture:         svc.Desc.Architecture,
		Tag:                  svc.Tag,
		AutoHealing:          svc.AutoHealing,
		AutoScaling:          svc.AutoScaling,
		HighAvailable:        svc.HighAvailable,
		Status:               svc.Status,
		BackupMaxSizeByte:    svc.BackupMaxSizeByte,
		BackupFilesRetention: svc.BackupFilesRetention,
		CreatedAt:            svc.CreatedAt.String(),
		FinishedAt:           svc.FinishedAt.String(),
	}
}

func convertStructsService(spec structs.ServiceSpec) (database.Service, error) {
	vb, err := json.Marshal(spec.Require.Volumes)
	if err != nil {
		return database.Service{}, errors.WithStack(err)
	}

	var nw = struct {
		NetworkingIDs []string
		Require       []structs.NetDeviceRequire
	}{
		spec.Networkings,
		spec.Require.Networks,
	}

	nwb, err := json.Marshal(nw)
	if err != nil {
		return database.Service{}, errors.WithStack(err)
	}

	arch, err := json.Marshal(spec.Arch)
	if err != nil {
		return database.Service{}, errors.WithStack(err)
	}
	opts, err := json.Marshal(spec.Options)
	if err != nil {
		return database.Service{}, errors.WithStack(err)
	}

	desc := database.ServiceDesc{
		ID:           utils.Generate32UUID(),
		ServiceID:    spec.ID,
		Architecture: string(arch),
		Replicas:     spec.Arch.Replicas,
		NCPU:         spec.Require.Require.CPU,
		Memory:       spec.Require.Require.Memory,
		Image:        spec.Image,
		Volumes:      string(vb),
		Networks:     string(nwb),
		Clusters:     strings.Join(spec.Clusters, ","),
		Options:      string(opts),
		Previous:     "",
	}

	return database.Service{
		ID:                   spec.ID,
		Name:                 spec.Name,
		DescID:               desc.ID,
		Tag:                  spec.Tag,
		AutoHealing:          spec.AutoHealing,
		AutoScaling:          spec.AutoScaling,
		HighAvailable:        spec.HighAvailable,
		Status:               spec.Status,
		BackupMaxSizeByte:    spec.BackupMaxSizeByte,
		BackupFilesRetention: spec.BackupFilesRetention,
		CreatedAt:            time.Now(),
		Desc:                 &desc,
	}, nil
}

func covertUnitNetwork(ips []database.IP) []structs.UnitIP {
	if len(ips) == 0 {
		return nil
	}

	out := make([]structs.UnitIP, 0, len(ips))

	for i := range ips {
		out = append(out, structs.UnitIP{
			Prefix:     ips[i].Prefix,
			VLAN:       ips[i].VLAN,
			Bandwidth:  ips[i].Bandwidth,
			Device:     ips[i].Bond,
			IP:         utils.Uint32ToIP(ips[i].IPAddr).String(),
			Gateway:    ips[i].Gateway,
			Networking: ips[i].Networking,
		})
	}

	return out

}

// TODO:convert to structs.UnitSpec
func convertUnitInfoToSpec(info database.UnitInfo, container *cluster.Container) structs.UnitSpec {
	lvs := make([]structs.VolumeSpec, 0, len(info.Volumes))

	for i := range info.Volumes {
		lvs = append(lvs, structs.VolumeSpec{
			ID:      info.Volumes[i].ID,
			Name:    info.Volumes[i].Name,
			Type:    info.Volumes[i].VG,
			Driver:  info.Volumes[i].Driver,
			Size:    int(info.Volumes[i].Size),
			Options: map[string]interface{}{"fstype": info.Volumes[i].Filesystem},
		})
	}

	spec := structs.UnitSpec{
		Unit: structs.Unit(info.Unit),

		Engine: struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Addr string `json:"addr"`
		}{
			ID:   info.Engine.EngineID,
			Name: "",
			Addr: info.Engine.Addr,
		},

		Networking: covertUnitNetwork(info.Networkings),

		Volumes: lvs,
	}

	if container != nil {
		spec.Config = container.Config
		spec.Container = container.Container
		spec.Engine.Name = container.Engine.Name
	}

	return spec
}

func ConvertServiceInfo(info database.ServiceInfo, containers cluster.Containers) structs.ServiceSpec {
	units := make([]structs.UnitSpec, 0, len(info.Units))

	for u := range info.Units {
		var c *cluster.Container
		if containers != nil {
			c = containers.Get(info.Units[u].Unit.Name)
		}
		units = append(units, convertUnitInfoToSpec(info.Units[u], c))
	}

	arch := structs.Arch{}
	r := strings.NewReader(info.Service.Desc.Architecture)
	json.NewDecoder(r).Decode(&arch)

	var opts map[string]interface{}
	r1 := strings.NewReader(info.Service.Desc.Options)
	json.NewDecoder(r1).Decode(&opts)

	return structs.ServiceSpec{
		Arch:    arch,
		Service: convertService(info.Service),
		Units:   units,
		Options: opts,
	}
}

func (gd *Garden) GetService(nameOrID string) (*Service, error) {
	s, err := gd.ormer.GetService(nameOrID)
	if err != nil {
		return nil, err
	}

	svc := gd.NewService(nil, &s)

	return svc, nil
}

func (gd *Garden) Service(nameOrID string) (*Service, error) {
	info, err := gd.ormer.GetServiceInfo(nameOrID)
	if err != nil {
		return nil, err
	}

	spec := ConvertServiceInfo(info, gd.Cluster.Containers())

	svc := gd.NewService(&spec, &info.Service)

	return svc, nil
}

func (gd *Garden) ServiceSpec(nameOrID string) (structs.ServiceSpec, error) {
	info, err := gd.ormer.GetServiceInfo(nameOrID)
	if err != nil {
		return structs.ServiceSpec{}, err
	}

	spec := ConvertServiceInfo(info, gd.Cluster.Containers())

	return spec, nil
}

func (gd *Garden) ListServices(ctx context.Context) ([]structs.ServiceSpec, error) {
	list, err := gd.ormer.ListServicesInfo()
	if err != nil {
		return nil, err
	}

	containers := gd.Cluster.Containers()

	out := make([]structs.ServiceSpec, 0, len(list))

	for i := range list {
		spec := ConvertServiceInfo(list[i], containers)
		out = append(out, spec)
	}

	return out, nil
}

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

func (gd *Garden) validServiceSpec(spec structs.ServiceSpec) error {
	if spec.Arch.Replicas == 0 {
		return errors.New("replicas==0")
	}

	if spec.Require == nil {
		return errors.New("require UnitRequire")
	}

	return nil
}

func (gd *Garden) BuildService(spec structs.ServiceSpec) (*Service, *database.Task, error) {
	err := gd.validServiceSpec(spec)
	if err != nil {
		return nil, nil, err
	}

	options := scheduleOption{
		highAvailable: spec.HighAvailable,
		require:       *spec.Require,
	}

	options.nodes.constraints = spec.Constraints
	options.nodes.networkings = spec.Networkings
	options.nodes.clusters = spec.Clusters

	im, err := gd.ormer.GetImageVersion(spec.Image)
	if err != nil {
		return nil, nil, err
	}
	if spec.ID == "" {
		spec.ID = utils.Generate32UUID()
	}
	svc, err := convertStructsService(spec)
	if err != nil {
		return nil, nil, err
	}
	svc.Desc.ImageID = im.ID
	svc.Status = statusServcieBuilding

	us := make([]database.Unit, spec.Arch.Replicas)
	units := make([]structs.UnitSpec, spec.Arch.Replicas)

	netDesc, err := json.Marshal(spec.Require.Networks)
	if err != nil {
		return nil, nil, err
	}

	for i := 0; i < spec.Arch.Replicas; i++ {

		uid := utils.Generate32UUID()
		us[i] = database.Unit{
			ID:          uid,
			Name:        fmt.Sprintf("%s_%s", spec.Name, uid[:8]), // <service_name>_<unit_id_8bit>
			Type:        im.Name,
			ServiceID:   spec.ID,
			NetworkMode: "none",
			Networks:    string(netDesc),
			Status:      0,
			CreatedAt:   time.Now(),
		}

		units[i].Unit = structs.Unit(us[i])
	}

	spec.Units = units

	t := database.NewTask(spec.Name, database.ServiceRunTask, spec.ID, svc.Desc.ID, "", 300)

	err = gd.ormer.InsertService(svc, us, &t)
	if err != nil {
		return nil, nil, err
	}

	service := gd.NewService(&spec, &svc)

	service.options = options

	return service, &t, nil
}

type scheduleOption struct {
	highAvailable bool

	require structs.UnitRequire

	nodes struct {
		networkings []string
		clusters    []string
		filters     []string
		constraints []string
	}

	scheduler struct {
		strategy string
		filters  []string
	}
}

func (gd *Garden) schedule(ctx context.Context, actor allocator, config *cluster.ContainerConfig, opts scheduleOption) ([]*node.Node, error) {
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

	out, err := actor.ListCandidates(opts.nodes.clusters, opts.nodes.filters, opts.require.Volumes)
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
	volumes     []volume.VolumesCreateBody
}

func (pu pendingUnit) convertToSpec() structs.UnitSpec {
	return structs.UnitSpec{}
}

func (gd *Garden) Allocation(ctx context.Context, actor allocator, svc *Service) ([]pendingUnit, error) {
	ready := make([]pendingUnit, 0, svc.svc.Desc.Replicas)

	action := func() (err error) {
		_, version, err := getImage(gd.Ormer(), svc.svc.Desc.Image)
		if err != nil {
			return err
		}

		opts := svc.options
		config := cluster.BuildContainerConfig(container.Config{
			Tty:       true,
			OpenStdin: true,
			Image:     version,
		}, container.HostConfig{
			NetworkMode: "none",
			Binds:       []string{"/etc/localtime:/etc/localtime:ro"},
			Resources: container.Resources{
				CpusetCpus: strconv.Itoa(opts.require.Require.CPU),
				Memory:     opts.require.Require.Memory,
			},
		}, network.NetworkingConfig{})

		{
			for i := range opts.nodes.constraints {
				config.AddConstraint(opts.nodes.constraints[i])
			}
			if len(opts.nodes.filters) > 0 {
				config.AddConstraint(nodeLabel + "!=" + strings.Join(opts.nodes.filters, "|"))
			}
			if out := opts.nodes.clusters; len(out) > 0 {
				config.AddConstraint(clusterLabel + "==" + strings.Join(out, "|"))
			}
		}

		gd.Lock()
		defer gd.Unlock()

		gd.scheduler.Lock()
		nodes, err := gd.schedule(ctx, actor, config, opts)
		if err != nil {
			gd.scheduler.Unlock()
			return err
		}
		gd.scheduler.Unlock()

		if len(nodes) < svc.svc.Desc.Replicas {
			return errors.Errorf("not enough nodes for allocation,%d<%d", len(nodes), svc.svc.Desc.Replicas)
		}

		units, err := svc.so.ListUnitByServiceID(svc.svc.ID)
		if err != nil {
			return err
		}

		var (
			count = svc.svc.Desc.Replicas
			bad   = make([]pendingUnit, 0, count)
			used  = make([]*node.Node, 0, count)
			field = logrus.WithField("Service", svc.svc.Name)
		)

		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf("panic:%v", r)
			}
			// cancel allocation
			if len(bad) > 0 {
				_err := actor.RecycleResource()
				if _err != nil {
					field.Errorf("Recycle resources error:%+v", _err)
					err = fmt.Errorf("%+v\nRecycle resources error:%+v", err, _err)
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
			return ctx.Err()
		}

		for n := range nodes {

			if !selectNodeInDifferentCluster(opts.highAvailable, svc.svc.Desc.Replicas, nodes[n], used) {
				field.Debugf("highAvailable=%t node=%s,continue", opts.highAvailable, nodes[n].Name)
				continue
			}

			pu := pendingUnit{
				swarmID:     units[count-1].ID,
				Unit:        units[count-1],
				config:      config.DeepCopy(),
				networkings: make([]database.IP, 0, len(units)),
				volumes:     make([]volume.VolumesCreateBody, 0, len(units)),
			}

			_, err := actor.AlloctCPUMemory(pu.config, nodes[n], int64(opts.require.Require.CPU), config.HostConfig.Memory, nil)
			if err != nil {
				field.Debugf("AlloctCPUMemory:node=%s,%+v", nodes[n].Name, err)
				continue
			}

			networkings, err := actor.AlloctNetworking(pu.config, nodes[n].ID, pu.Unit.ID, opts.nodes.networkings, opts.require.Networks)
			if len(networkings) > 0 {
				pu.networkings = append(pu.networkings, networkings...)
			}
			if err != nil {
				bad = append(bad, pu)
				field.Debugf("AlloctNetworking:node=%s,%+v", nodes[n].Name, err)
				continue
			}

			volumes, err := actor.AlloctVolumes(pu.config, pu.Unit.ID, nodes[n], opts.require.Volumes)
			if len(volumes) > 0 {
				pu.volumes = append(pu.volumes, volumes...)
			}
			if err != nil {
				bad = append(bad, pu)
				field.Debugf("AlloctVolumes:node=%s,%+v", nodes[n].Name, err)
				continue
			}

			pu.config.SetSwarmID(pu.swarmID)
			pu.Unit.EngineID = nodes[n].ID

			err = gd.Cluster.AddPendingContainer(pu.Name, pu.swarmID, nodes[n].ID, pu.config)
			if err != nil {
				field.Debugf("AddPendingContainer:node=%s,%+v", nodes[n].ID, err)
				continue
			}

			ready = append(ready, pu)
			used = append(used, nodes[n])

			if count--; count == 0 {
				break
			}
		}

		if count > 0 {
			bad = append(bad, ready...)
			return errors.Errorf("not enough nodes for allocation,%d units waiting", count)
		}

		return err
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceAllocating, statusServiceAllocated, statusServiceAllocateFailed)

	err := sl.Run(
		func(val int) bool {
			return val == statusServcieBuilding
		},
		action)

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
