package garden

import (
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
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// var errConvertServiceSpec = stderr.New("convert structs.ServiceSpec to database.Service")

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

func (svc Service) getUnit(nameOrID string) (*unit, error) {
	u, err := svc.so.GetUnit(nameOrID)
	if err != nil {
		return nil, err
	}

	if u.ServiceID != svc.svc.ID {
		return nil, nil
	}

	return newUnit(u, svc.so, svc.cluster), nil
}

func (svc Service) getUnits() ([]*unit, error) {
	list, err := svc.so.ListUnitByServiceID(svc.svc.ID)
	if err != nil {
		return nil, err
	}

	units := make([]*unit, len(list))

	for i := range list {
		units[i] = newUnit(list[i], svc.so, svc.cluster)
	}

	return units, nil
}

func (svc *Service) Spec() (*structs.ServiceSpec, error) {
	if svc.spec != nil {
		return svc.spec, nil
	}

	if svc.svc != nil && svc.so != nil {

		var containers cluster.Containers
		if svc.cluster != nil {
			containers = svc.cluster.Containers()
		}

		info, err := svc.so.GetServiceInfo(svc.svc.ID)
		if err != nil {
			return nil, err
		}

		spec := ConvertServiceInfo(info, containers)
		svc.spec = &spec

		return svc.spec, nil
	}

	return nil, errors.New("Service internal error")
}

func (svc *Service) RefreshSpec() (*structs.ServiceSpec, error) {
	var (
		ID    string
		users []structs.User
	)

	if svc.svc != nil {
		ID = svc.svc.ID
	} else if svc.spec != nil {
		ID = svc.spec.ID
		users = svc.spec.Users
	} else {
		return nil, errors.New("Service with non ID")
	}

	if svc.so == nil {
		return nil, errors.New("Service internal error")
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

	return &spec, nil
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
		var typ string
		if len(parts) >= 2 {
			typ = parts[len(parts)-2]
		}
		lvs = append(lvs, structs.VolumeSpec{
			ID:      info.Volumes[i].ID,
			Name:    info.Volumes[i].Name,
			Type:    typ,
			Driver:  info.Volumes[i].Driver,
			Size:    int(info.Volumes[i].Size),
			Options: map[string]interface{}{"fstype": info.Volumes[i].Filesystem},
		})
	}

	spec := structs.UnitSpec{
		Unit: structs.Unit(info.Unit),

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

func (svc *Service) RunContainer(ctx context.Context, pendings []pendingUnit, authConfig *types.AuthConfig) (err error) {
	defer func() {
		ids := make([]string, len(pendings))
		for i := range pendings {
			ids[i] = pendings[i].swarmID
		}
		svc.cluster.RemovePendingContainer(ids...)
	}()

	run := func() error {
		select {
		default:
		case <-ctx.Done():
			return ctx.Err()
		}

		for _, pu := range pendings {
			eng := svc.cluster.Engine(pu.Unit.EngineID)
			if eng == nil {
				return nil
			}

			for _, lv := range pu.volumes {

				v := volume.VolumesCreateBody{
					Name:   lv.Name,
					Driver: lv.Driver,
					Labels: nil,
					DriverOpts: map[string]string{
						"size":   strconv.Itoa(int(lv.Size)),
						"fstype": lv.Filesystem,
						"vgname": lv.VG,
					},
				}

				_, err := eng.CreateVolume(&v)
				if err != nil {
					return err
				}
			}

			c, err := eng.CreateContainer(pu.config, pu.Unit.Name, true, authConfig)
			if err != nil {
				return err
			}
			pu.Unit.ContainerID = c.ID

			err = eng.StartContainer(c, nil)
			if err != nil {
				return errors.Wrap(err, "start container:"+pu.Unit.Name)
			}
		}

		return nil
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceContainerCreating, statusServiceContainerRunning, statusServiceContainerCreateFailed)

	return sl.Run(func(val int) bool {
		return val == statusServiceAllocated
	}, run, false)
}

func (svc *Service) InitStart(ctx context.Context, kvc kvstore.Client, configs structs.ConfigsMap, task *database.Task, async bool, args map[string]interface{}) error {

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, task,
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
		return svc.initStart(ctx, kvc, configs, args)
	}, async)
}

func (svc *Service) initStart(ctx context.Context, kvc kvstore.Client, configs structs.ConfigsMap, args map[string]interface{}) error {
	units, err := svc.getUnits()
	if err != nil {
		return err
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

		_, err = units[i].containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}
	}

	return registerUnits(ctx, units, kvc, configs)
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

		err = saveContainerToKV(kvc, u.getContainer())
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

func saveContainerToKV(kvc kvstore.Client, c *cluster.Container) error {
	if kvc == nil || c == nil {
		return nil
	}

	val, err := json.Marshal(c)
	if err != nil {
		return errors.Wrapf(err, "JSON marshal Container %s", c.Info.Name)
	}

	const containerKV = "/containers/"

	err = kvc.PutKV(containerKV+c.ID, val)

	return err
}

func (svc *Service) Start(ctx context.Context, task *database.Task, detach bool, cmds structs.Commands) error {

	start := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
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

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, task,
		statusServiceStarting, statusServiceStarted, statusServiceStartFailed)

	return sl.Run(isnotInProgress, start, detach)
}

func (svc *Service) UpdateUnitsConfigs(ctx context.Context, configs structs.ConfigsMap, args map[string]interface{}, task *database.Task, async bool) (err error) {

	update := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		return svc.updateConfigs(ctx, units, configs, args)
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, task,
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

func (svc *Service) UpdateConfig(ctx context.Context, nameOrID string, args map[string]interface{}) error {
	u, err := svc.getUnit(nameOrID)
	if err != nil {
		return err
	}

	config, err := svc.generateUnitConfig(ctx, u.u.ID, args)
	if err != nil {
		return err
	}

	err = u.updateServiceConfig(ctx, config.DataMount, config.Content)

	return err
}

func (svc *Service) Stop(ctx context.Context, containers, async bool, task *database.Task) error {

	stop := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		return svc.stop(ctx, units, containers)
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, task,
		statusServiceStoping, statusServiceStoped, statusServiceStopFailed)

	return sl.Run(isnotInProgress, stop, async)
}

func (svc *Service) stop(ctx context.Context, units []*unit, containers bool) error {
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

func (svc *Service) Exec(ctx context.Context, config structs.ServiceExecConfig, async bool, task *database.Task) error {

	exec := func() error {
		if config.Container != "" {
			_, err := svc.exec(ctx, config.Container, config.Cmd, config.Detach)
			return err
		}

		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		for i := range units {
			_, err := svc.exec(ctx, units[i].u.ID, config.Cmd, config.Detach)
			if err != nil {
				return err
			}
		}

		return nil
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, task,
		statusServiceExecStart, statusServiceExecDone, statusServiceExecFailed)

	return sl.Run(isnotInProgress, exec, async)
}

func (svc *Service) exec(ctx context.Context, nameOrID string, cmd []string, detach bool) (types.ContainerExecInspect, error) {
	u, err := svc.getUnit(nameOrID)
	if err != nil {
		// exec if container exist
		c := svc.cluster.Container(nameOrID)
		if c != nil {
			return c.Exec(ctx, cmd, detach)
		}

		return types.ContainerExecInspect{}, err
	}

	return u.containerExec(ctx, cmd, detach)
}

func (svc *Service) Remove(ctx context.Context, r kvstore.Register) (err error) {
	err = svc.deleteCondition()
	if err != nil {
		return err
	}

	remove := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		select {
		default:
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		}

		err = svc.deregisterSerivces(ctx, r, units)
		if err != nil {
			return err
		}

		err = svc.removeContainers(ctx, units, true, false)
		if err != nil {
			return err
		}

		err = svc.removeVolumes(ctx, units)
		if err != nil {
			return err
		}

		err = svc.so.DelServiceRelation(svc.svc.ID, true)

		return err
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceDeleting, 0, statusServiceDeleteFailed)

	sl.SetAfter(func(key string, val int, task *database.Task, t time.Time) error {
		err := svc.so.SetServiceWithTask(key, val, task, t)
		if err != nil {
			logrus.WithField("Service", svc.svc.Name).Warnf("remove Service:%+v", err)
		}
		if err != nil && task != nil {
			return svc.so.SetTask(*task)
		}

		return nil
	})

	return sl.Run(isnotInProgress, remove, false)
}

func (svc *Service) removeContainers(ctx context.Context, units []*unit, force, rmVolumes bool) error {

	for _, u := range units {
		engine := u.getEngine()
		if engine == nil {
			continue
		}

		if c := u.getContainer(); c == nil {
			id := u.u.ContainerID
			if id == "" {
				id = u.u.Name
			}
			err := engine.RemoveContainer(&cluster.Container{
				Container: types.Container{ID: id}}, force, rmVolumes)
			if err != nil {
				logrus.WithField("Service", svc.svc.Name).Errorf("remove container:%s in engine %s,%+v", id, engine.Addr, err)
			}
			continue
		}

		client := engine.ContainerAPIClient()
		if client == nil {
			continue
		}

		if !force {
			timeout := 30 * time.Second
			err := client.ContainerStop(ctx, u.u.Name, &timeout)
			engine.CheckConnectionErr(err)
			if err != nil {
				return err
			}
		}

		options := types.ContainerRemoveOptions{
			RemoveVolumes: rmVolumes,
			RemoveLinks:   false,
			Force:         force,
		}
		err := client.ContainerRemove(ctx, u.u.Name, options)
		engine.CheckConnectionErr(err)
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

func (svc Service) deregisterSerivces(ctx context.Context, reg kvstore.Register, units []*unit) error {
	for i := range units {

		err := reg.DeregisterService(ctx, structs.ServiceDeregistration{
			Type: "units",
			Key:  units[i].u.ID,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

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

func (svc Service) Image() (database.Image, error) {
	img, err := structs.ParseImage(svc.svc.Desc.Image)
	if err != nil {
		return database.Image{}, err
	}

	return svc.so.GetImage(img.Name, img.Major, img.Minor, img.Patch)
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

func (svc *Service) GenerateUnitsConfigs(ctx context.Context, args map[string]interface{}) (structs.ConfigsMap, error) {
	return svc.generateUnitsConfigs(ctx, args)
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
	return svc.pc.GetCommands(ctx, svc.svc.ID)
}
