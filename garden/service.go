package garden

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
	"github.com/docker/swarm/vars"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Service is exported.
type Service struct {
	so      database.ServiceOrmer
	pc      pluginapi.PluginAPI
	svc     *database.Service
	spec    *structs.ServiceSpec
	cluster cluster.Cluster

	options scheduleOption
}

func newService(spec *structs.ServiceSpec,
	svc *database.Service,
	so database.ServiceOrmer,
	cluster cluster.Cluster,
	pc pluginapi.PluginAPI) *Service {

	return &Service{
		spec:    spec,
		svc:     svc,
		so:      so,
		cluster: cluster,
		pc:      pc,
	}
}

// ID returns Service ID
func (svc Service) ID() string {
	if svc.svc != nil {
		return svc.svc.ID
	}

	if svc.spec != nil {
		return svc.spec.ID
	}

	return ""
}

// Name returns Service Name
func (svc Service) Name() string {
	if svc.svc != nil {
		return svc.svc.Name
	}

	if svc.spec != nil {
		return svc.spec.Name
	}

	return ""
}

// GetUnit returns unit by name or id
func (svc Service) GetUnit(nameOrID string) (*unit, error) {
	return svc.getUnit(nameOrID)
}

func (svc Service) getUnit(nameOrID string) (*unit, error) {
	u, err := svc.so.GetUnit(nameOrID)
	if err != nil {
		return nil, err
	}

	if u.ServiceID != svc.ID() {
		return nil, nil
	}

	return newUnit(u, svc.so, svc.cluster), nil
}

func (svc Service) getUnits() ([]*unit, error) {
	list, err := svc.so.ListUnitByServiceID(svc.ID())
	if err != nil {
		return nil, err
	}

	units := make([]*unit, len(list))

	for i := range list {
		units[i] = newUnit(list[i], svc.so, svc.cluster)
	}

	return units, nil
}

func getUnit(units []*unit, nameOrID string) *unit {
	for i := range units {
		if units[i].u.ID == nameOrID ||
			units[i].u.Name == nameOrID ||
			units[i].u.ContainerID == nameOrID {

			return units[i]
		}
	}

	return nil
}

// Spec returns ServiceSpec,if nil,query ServiceInfo from db,then convert to ServiceSpec
func (svc *Service) Spec() (*structs.ServiceSpec, error) {
	if svc.spec != nil {
		return svc.spec, nil
	}

	if svc.svc != nil && svc.so != nil {

		var containers cluster.Containers
		if svc.cluster != nil {
			containers = svc.cluster.Containers()
		}

		info, err := svc.so.GetServiceInfo(svc.ID())
		if err != nil {
			return nil, err
		}

		spec := ConvertServiceInfo(info, containers)
		svc.spec = &spec

		return svc.spec, nil
	}

	return nil, errors.New("Service internal error")
}

// RefreshSpec query from db,convert to ServiceSpec
func (svc *Service) RefreshSpec() (*structs.ServiceSpec, error) {
	var (
		ID    string
		users []structs.User
	)

	if svc == nil || svc.so == nil {
		return nil, errors.New("Service internal error")
	}

	if svc.spec != nil {
		ID = svc.spec.ID
		users = svc.spec.Users
	} else if svc.svc != nil {
		ID = svc.ID()
	} else {
		return nil, errors.New("Service with non ID")
	}

	var containers cluster.Containers
	if svc.cluster != nil {
		containers = svc.cluster.Containers()
	}

	info, err := svc.so.GetServiceInfo(ID)
	if err != nil {
		return nil, err
	}

	spec := ConvertServiceInfo(info, containers)
	svc.spec = &spec
	svc.svc = &info.Service

	if spec.Users == nil && users != nil {
		spec.Users = users
	}

	return svc.spec, nil
}

func convertService(svc database.Service) structs.Service {
	if svc.Desc == nil {
		svc.Desc = &database.ServiceDesc{}
	}

	return structs.Service{
		ID:            svc.ID,
		Name:          svc.Name,
		Image:         svc.Desc.Image,
		Desc:          svc.DescID,
		Architecture:  svc.Desc.Architecture,
		Tag:           svc.Tag,
		AutoHealing:   svc.AutoHealing,
		AutoScaling:   svc.AutoScaling,
		HighAvailable: svc.HighAvailable,
		Status:        svc.Status,
		CreatedAt:     svc.CreatedAt.String(),
		FinishedAt:    svc.FinishedAt.String(),
	}
}

func convertStructsService(spec structs.ServiceSpec, schedopts scheduleOption) (database.Service, error) {
	vb, err := json.Marshal(spec.Require.Volumes)
	if err != nil {
		return database.Service{}, errors.WithStack(err)
	}

	var nw = struct {
		NetworkingIDs map[string][]string
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
	schd, err := json.Marshal(schedopts)
	if err != nil {
		return database.Service{}, errors.WithStack(err)
	}

	desc := database.ServiceDesc{
		ID:              utils.Generate32UUID(),
		ServiceID:       spec.ID,
		Architecture:    string(arch),
		ScheduleOptions: string(schd),
		Replicas:        spec.Arch.Replicas,
		NCPU:            spec.Require.Require.CPU,
		Memory:          spec.Require.Require.Memory,
		Image:           spec.Image,
		Volumes:         string(vb),
		Networks:        string(nwb),
		Clusters:        strings.Join(spec.Clusters, ","),
		Options:         string(opts),
		Previous:        "",
	}

	return database.Service{
		ID:            spec.ID,
		Name:          spec.Name,
		DescID:        desc.ID,
		Tag:           spec.Tag,
		AutoHealing:   spec.AutoHealing,
		AutoScaling:   spec.AutoScaling,
		HighAvailable: spec.HighAvailable,
		Status:        spec.Status,
		CreatedAt:     time.Now(),
		Desc:          &desc,
	}, nil
}

func covertUnitNetwork(ips []database.IP) []structs.UnitIP {
	if len(ips) == 0 {
		return []structs.UnitIP{}
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

func convertUnitInfoToSpec(info database.UnitInfo, container *cluster.Container) structs.UnitSpec {
	lvs := make([]structs.VolumeSpec, 0, len(info.Volumes))

	for i := range info.Volumes {
		parts := strings.Split(info.Volumes[i].Name, "_")

		lvs = append(lvs, structs.VolumeSpec{
			ID:      info.Volumes[i].ID,
			Name:    info.Volumes[i].Name,
			Type:    parts[len(parts)-1],
			Driver:  info.Volumes[i].Driver,
			Size:    int(info.Volumes[i].Size),
			Options: map[string]interface{}{"fstype": info.Volumes[i].Filesystem},
		})
	}

	spec := structs.UnitSpec{
		Unit: structs.Unit(info.Unit),

		Config: &cluster.ContainerConfig{},

		Engine: struct {
			ID   string `json:"id"`
			Node string `json:"node"`
			Name string `json:"name"`
			Addr string `json:"addr"`
		}{
			ID:   info.Engine.EngineID,
			Node: info.Engine.ID,
			Addr: info.Engine.Addr,
		},

		Networking: covertUnitNetwork(info.Networkings),

		Volumes: lvs,
	}

	if container != nil {
		spec.Config = container.Config
		spec.Container = container.Container
		spec.Engine.ID = container.Engine.ID
		spec.Engine.Name = container.Engine.Name
		spec.Engine.Addr = container.Engine.IP
	}

	return spec
}

func getUnitContainer(containers cluster.Containers, u database.Unit) *cluster.Container {
	if len(containers) == 0 {
		return nil
	}

	var c *cluster.Container

	if u.ContainerID != "" {
		c = containers.Get(u.ContainerID)
	}

	if c == nil {
		c = containers.Get(u.Name)
	}

	if c == nil {
		c = containers.Get(u.ID)
	}

	if c == nil {
		logrus.Warnf("not found Container by '%s' & '%s' & '%s'", u.ContainerID, u.Name, u.ID)
	}

	return c
}

// ConvertServiceInfo returns ServiceSpec,covert by ServiceInfo and Containers
func ConvertServiceInfo(info database.ServiceInfo, containers cluster.Containers) structs.ServiceSpec {
	units := make([]structs.UnitSpec, 0, len(info.Units))

	for u := range info.Units {
		c := getUnitContainer(containers, info.Units[u].Unit)

		units = append(units, convertUnitInfoToSpec(info.Units[u], c))
	}

	var (
		arch     structs.Arch
		opts     map[string]interface{}
		scheOpts scheduleOption
	)

	if info.Service.Desc != nil {
		r := strings.NewReader(info.Service.Desc.Architecture)
		json.NewDecoder(r).Decode(&arch)

		r = strings.NewReader(info.Service.Desc.Options)
		json.NewDecoder(r).Decode(&opts)

		r = strings.NewReader(info.Service.Desc.ScheduleOptions)

		json.NewDecoder(r).Decode(&scheOpts)
	}

	return structs.ServiceSpec{
		Arch:    arch,
		Service: convertService(info.Service),
		Require: &scheOpts.Require,
		Units:   units,
		Options: opts,
	}
}

// RunContainer create and start container on engine.
func (svc *Service) RunContainer(ctx context.Context, pendings []pendingUnit, start bool, authConfig *types.AuthConfig) error {
	sl := tasklock.NewServiceTask(svc.ID(), svc.so, nil,
		statusServiceContainerCreating, statusServiceContainerRunning, statusServiceContainerCreateFailed)

	return sl.Run(
		func(val int) bool {
			return val == statusServiceAllocated
		},
		func() error {
			return svc.runContainer(ctx, pendings, start, authConfig)
		},
		false)
}

func (svc *Service) runContainer(ctx context.Context, pendings []pendingUnit, start bool, authConfig *types.AuthConfig) error {
	defer func() {
		ids := make([]string, len(pendings))
		for i := range pendings {
			ids[i] = pendings[i].swarmID
		}
		svc.cluster.RemovePendingContainer(ids...)
	}()

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	for _, pu := range pendings {
		eng := svc.cluster.Engine(pu.Unit.EngineID)
		if eng == nil {
			return errors.Errorf("Engine '%s':no long exist", pu.Unit.EngineID)
		}

		for _, lv := range pu.volumes {
			err := engineCreateVolume(eng, lv)
			if err != nil {
				return err
			}
		}

		c, err := eng.CreateContainer(pu.config, pu.Unit.Name, true, authConfig)
		if err != nil {
			return err
		}
		pu.Unit.ContainerID = c.ID

		if start {
			err = eng.StartContainer(c)
			if err != nil {
				return errors.Wrap(err, "start container:"+pu.Unit.Name)
			}
		}
	}

	return nil
}

func engineCreateVolume(eng *cluster.Engine, lv database.Volume) error {
	body := volume.VolumesCreateBody{
		Name:   lv.Name,
		Driver: lv.Driver,
		Labels: nil,
		DriverOpts: map[string]string{
			"size":   strconv.Itoa(int(lv.Size)),
			"fstype": lv.Filesystem,
			"vgname": lv.VG,
		},
	}

	_, err := eng.CreateVolume(&body)
	if err == nil {
		return nil
	}

	return errors.WithStack(err)
}

// InitStart start container & exec start service command,exec init-start service command if the first start.
// register services to consul or other third part auto-service discovery server.
func (svc *Service) InitStart(ctx context.Context, kvc kvstore.Client, configs structs.ConfigsMap, task *database.Task, async bool, args map[string]interface{}) error {

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, task,
		statusInitServiceStarting, statusInitServiceStarted, statusInitServiceStartFailed)

	val, err := sl.Load()
	if err == nil {
		if val > statusInitServiceStartFailed {
			return svc.Start(ctx, task, async, configs.Commands())
		}
	}

	check := func(val int) bool {
		if val == statusServiceContainerRunning || val == statusServiceUnitMigrating {
			return true
		}
		return false
	}

	return sl.Run(check, func() error {
		return svc.initStart(ctx, nil, kvc, configs, args)
	}, async)
}

func (svc *Service) initStart(ctx context.Context, units []*unit, kvc kvstore.Client, configs structs.ConfigsMap, args map[string]interface{}) (err error) {
	if units == nil {
		units, err = svc.getUnits()
		if err != nil {
			return err
		}
	}

	if configs == nil {
		configs, err = svc.generateUnitsConfigs(ctx, args)
		if err != nil {
			return err
		}
	}

	// start containers and update configs
	err = svc.updateConfigs(ctx, units, configs, args)
	if err != nil {
		return err
	}

	for i := range units {
		cmd := configs.GetCmd(units[i].u.ID, structs.InitServiceCmd)
		root := vars.Root
		mon := vars.Monitor
		repl := vars.Replication
		chk := vars.Check
		cmd = append(cmd, root.Role, root.User, root.Password,
			mon.Role, mon.User, mon.Password,
			repl.Role, repl.User, repl.Password,
			chk.Role, chk.User, chk.Password)

		_, err = units[i].containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}
	}

	if kvc != nil {
		return registerUnits(ctx, units, kvc, configs)
	}

	return nil
}

func registerUnits(ctx context.Context, units []*unit, kvc kvstore.Client, configs structs.ConfigsMap) error {
	if kvc == nil {
		return nil
	}

	// register to kv store and third-part services
	for _, u := range units {
		host, err := u.getHostIP()
		if err != nil {
			return err
		}

		err = saveContainerToKV(ctx, kvc, u.getContainer())
		if err != nil {
			return err
		}

		config, ok := configs.Get(u.u.ID)
		if !ok {
			return errors.Errorf("unit %s config is required", u.u.Name)
		}

		r := config.GetServiceRegistration()

		err = kvc.RegisterService(ctx, host, r)
		if err != nil {
			return err
		}
	}

	return nil
}

func saveContainerToKV(ctx context.Context, kvc kvstore.Client, c *cluster.Container) error {
	if kvc == nil || c == nil {
		return nil
	}

	val, err := json.Marshal(c)
	if err != nil {
		return errors.Wrapf(err, "JSON marshal Container %s", c.Info.Name)
	}

	err = kvc.PutKV(ctx, containerKV+c.ID, val)

	return err
}

func getContainerFromKV(ctx context.Context, kvc kvstore.Client, containerID string) (*cluster.Container, error) {
	if kvc == nil {
		return nil, errors.New("kvstore.Client is required")
	}

	pair, err := kvc.GetKV(ctx, containerKV+containerID)
	if err != nil {
		return nil, err
	}

	var c *cluster.Container

	buf := bytes.NewBuffer(pair.Value)

	err = json.NewDecoder(buf).Decode(&c)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (svc *Service) start(ctx context.Context, units []*unit, cmds structs.Commands) (err error) {
	if units == nil {
		units, err = svc.getUnits()
		if err != nil {
			return err
		}
	}

	if len(cmds) == 0 {
		cmds, err = svc.generateUnitsCmd(ctx)
		if err != nil {
			return err
		}
	}

	for i := range units {
		err = units[i].startContainer(ctx)
		if err != nil {
			return err
		}
	}

	// get start cmd
	for i := range units {
		cmd := cmds.GetCmd(units[i].u.ID, structs.StartServiceCmd)

		_, err = units[i].containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}
	}

	return nil
}

// Start start containers and services
func (svc *Service) Start(ctx context.Context, task *database.Task, detach bool, cmds structs.Commands) error {
	start := func() error {
		return svc.start(ctx, nil, cmds)
	}

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, task,
		statusServiceStarting, statusServiceStarted, statusServiceStartFailed)

	return sl.Run(isnotInProgress, start, detach)
}

// UpdateUnitsConfigs generated new units configs,write to container volume.
func (svc *Service) UpdateUnitsConfigs(ctx context.Context, configs structs.ServiceConfigs, task *database.Task, restart, async bool) (err error) {

	update := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		cm, err := svc.pc.UpdateConfigs(ctx, svc.ID(), configs)
		if err != nil {
			return err
		}

		err = svc.updateConfigs(ctx, units, cm, nil)
		if err != nil {
			return err
		}

		if restart {
			err = svc.start(ctx, units, cm.Commands())
		}

		return err
	}

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, task,
		statusServiceConfigUpdating, statusServiceConfigUpdated, statusServiceConfigUpdateFailed)

	return sl.Run(isnotInProgress, update, async)
}

// updateConfigs update units configurationFile,
// generate units configs if configs is nil,
// start units containers before update container configurationFile.
func (svc *Service) updateConfigs(ctx context.Context, units []*unit, configs structs.ConfigsMap, args map[string]interface{}) (err error) {
	if configs == nil {
		configs, err = svc.generateUnitsConfigs(ctx, args)
		if err != nil {
			return err
		}
	}

	for i := range units {
		err := units[i].startContainer(ctx)
		if err != nil {
			return err
		}
	}

	for i := range units {
		config, ok := configs.Get(units[i].u.ID)
		if !ok {
			continue
		}

		err := units[i].updateServiceConfig(ctx, config.ConfigFile, config.Content)
		if err != nil {
			return err
		}
	}

	return nil
}

// UpdateUnitConfig update the assigned unit config file.
func (svc *Service) UpdateUnitConfig(ctx context.Context, nameOrID, path, content string) error {
	u, err := svc.getUnit(nameOrID)
	if err != nil {
		return err
	}

	return u.updateServiceConfig(ctx, path, content)
}

// Stop stop units services,stop container if containers is true.
func (svc *Service) Stop(ctx context.Context, containers, async bool, task *database.Task) error {

	stop := func() error {
		return svc.stop(ctx, nil, containers)
	}

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, task,
		statusServiceStoping, statusServiceStoped, statusServiceStopFailed)

	return sl.Run(isnotInProgress, stop, async)
}

func (svc *Service) stop(ctx context.Context, units []*unit, containers bool) (err error) {
	if units == nil {
		units, err = svc.getUnits()
		if err != nil {
			return err
		}
	}

	cmds, err := svc.generateUnitsCmd(ctx)
	if err != nil {
		return err
	}

	for i := range units {
		cmd := cmds.GetCmd(units[i].u.ID, structs.StopServiceCmd)

		_, err = units[i].containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}
	}

	if !containers {
		return nil
	}

	for i := range units {
		err = units[i].stopContainer(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// Exec exec command in Service containers,if config.Container is assigned,exec the assigned unit command
func (svc *Service) Exec(ctx context.Context, config structs.ServiceExecConfig, async bool, task *database.Task) error {

	exec := func() error {
		var (
			err   error
			units []*unit
		)

		if config.Container != "" {
			var u *unit
			u, err = svc.getUnit(config.Container)
			units = []*unit{u}
		} else {
			units, err = svc.getUnits()
		}
		if err != nil {
			return err
		}

		for i := range units {
			_, err = units[i].containerExec(ctx, config.Cmd, config.Detach)
			if err != nil {
				return err
			}
		}

		return nil
	}

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, task,
		statusServiceExecStart, statusServiceExecDone, statusServiceExecFailed)

	return sl.Run(isnotInProgress, exec, async)
}

func (svc *Service) ExecLock(exec func() error, async bool, task *database.Task) error {
	sl := tasklock.NewServiceTask(svc.ID(), svc.so, task,
		statusServiceExecStart, statusServiceExecDone, statusServiceExecFailed)

	return sl.Run(isnotInProgress, exec, async)
}

// Remove remove Service,
// 1) deregiste services
// 2) remove containers
// 3) remove volumes
// 4) delete Service records in db
func (svc *Service) Remove(ctx context.Context, r kvstore.Register, force bool) (err error) {
	err = svc.deleteCondition()
	if err != nil {
		return err
	}

	remove := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		if !force && svc.svc.Status >= statusServiceContainerCreating {
			// check engines whether is alive before really delete
			for _, u := range units {
				if e := u.getEngine(); e == nil && u.u.EngineID != "" {
					return errors.Errorf("Engine %s is unhealthy", u.u.EngineID)
				}
			}
		}

		select {
		default:
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		}

		err = svc.deregisterServices(ctx, r, units)
		if err != nil {
			if force {
				logrus.WithField("Service", svc.Name()).Errorf("Service deregiste error:%+v", err)
			} else {
				return err
			}
		}

		err = svc.removeContainers(ctx, units, force, false)
		if err != nil {
			return err
		}

		err = svc.removeVolumes(ctx, units)
		if err != nil {
			return err
		}

		err = svc.so.DelServiceRelation(svc.ID(), true)

		return err
	}

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, nil,
		statusServiceDeleting, 0, statusServiceDeleteFailed)

	sl.After = func(key string, val int, task *database.Task, t time.Time) (err error) {
		if val == statusServiceDeleteFailed {
			err = svc.so.SetServiceWithTask(key, val, task, t)
		} else if task != nil {
			err = svc.so.SetTask(*task)
		}

		return err
	}

	return sl.Run(isnotInProgress, remove, false)
}

func (svc *Service) removeContainers(ctx context.Context, units []*unit, force, rmVolumes bool) error {
	for _, u := range units {
		err := u.removeContainer(ctx, rmVolumes, force)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) removeVolumes(ctx context.Context, units []*unit) error {

	for _, u := range units {
		err := u.removeVolumes(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc Service) deleteCondition() error {
	return nil
}

func (svc Service) deregisterServices(ctx context.Context, reg kvstore.Register, units []*unit) error {
	for i := range units {
		err := deregisterService(ctx, reg, "units", units[i].u.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func deregisterService(ctx context.Context, reg kvstore.Register, _type, key string) error {
	return reg.DeregisterService(ctx, structs.ServiceDeregistration{
		Type: _type,
		Key:  key,
	}, false)
}

func (svc *Service) removeUnits(ctx context.Context, rm []*unit, reg kvstore.Register) error {
	if reg != nil {
		err := svc.deregisterServices(ctx, reg, rm)
		if err != nil {
			return err
		}
	}

	err := svc.removeContainers(ctx, rm, true, false)
	if err != nil {
		return err
	}

	err = svc.removeVolumes(ctx, rm)
	if err != nil {
		return err
	}

	list := make([]database.Unit, 0, len(rm))
	for i := range rm {
		if rm[i] == nil {
			continue
		}

		list = append(list, rm[i].u)
	}

	err = svc.so.DelUnitsRelated(list, true)

	return err
}

// Compose call plugin compose
func (svc *Service) Compose(ctx context.Context, pc pluginapi.PluginAPI) error {
	var opts map[string]interface{}

	if svc.spec != nil {
		opts = svc.spec.Options
	}

	spec, err := svc.RefreshSpec()
	if err != nil {
		return err
	}

	spec.Options = opts

	return pc.ServiceCompose(ctx, *spec)
}

// Image returns Image,query from db.
func (svc Service) Image() (database.Image, error) {
	img, err := structs.ParseImage(svc.svc.Desc.Image)
	if err != nil {
		return database.Image{}, err
	}

	return svc.so.GetImage(img.Name, img.Major, img.Minor, img.Patch, img.Build)
}

func (svc *Service) generateUnitsConfigs(ctx context.Context, args map[string]interface{}) (structs.ConfigsMap, error) {
	if svc.spec != nil && len(svc.spec.Options) > 0 {

		for key, val := range args {
			svc.spec.Options[key] = val
		}

		args = svc.spec.Options
	}

	spec, err := svc.RefreshSpec()
	if err != nil {
		return nil, err
	}

	spec.Options = args

	return svc.pc.GenerateServiceConfig(ctx, *spec)
}

func (svc *Service) generateUnitConfig(ctx context.Context, nameOrID string, args map[string]interface{}) (structs.ConfigCmds, error) {
	if svc.spec != nil && len(svc.spec.Options) > 0 {

		for key, val := range args {
			svc.spec.Options[key] = val
		}

		args = svc.spec.Options
	}

	spec, err := svc.RefreshSpec()
	if err != nil {
		return structs.ConfigCmds{}, err
	}

	spec.Options = args

	return svc.pc.GenerateUnitConfig(ctx, nameOrID, *spec)
}

func (svc *Service) generateUnitsCmd(ctx context.Context) (structs.Commands, error) {
	return svc.pc.GetCommands(ctx, svc.ID())
}

func (svc *Service) GetUnitsConfigs(ctx context.Context) (structs.ServiceConfigs, error) {
	return svc.pc.GetServiceConfig(ctx, svc.ID())
}
