package garden

import (
	"encoding/json"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

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

func (gd *Garden) NewService(spec *structs.ServiceSpec, svc *database.Service) *Service {
	if spec == nil && svc == nil {
		return nil
	}

	return newService(spec, svc, gd.ormer, gd.Cluster, gd.pluginClient)
}

func (svc *Service) getUnit(nameOrID string) (*unit, error) {
	u, err := svc.so.GetUnit(nameOrID)
	if err != nil {
		return nil, err
	}

	if u.ServiceID != svc.svc.ID {
		return nil, nil
	}

	return newUnit(u, svc.so, svc.cluster), nil
}

func (svc *Service) getUnits() ([]*unit, error) {
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

func (svc Service) Spec() (*structs.ServiceSpec, error) {
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

func (svc Service) RefreshSpec() (*structs.ServiceSpec, error) {
	var ID string

	if svc.svc != nil {
		ID = svc.svc.ID
	} else if svc.spec != nil {
		ID = svc.spec.ID
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

	return &spec, nil
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

			for i := range pu.volumes {
				_, err := eng.CreateVolume(&pu.volumes[i])
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
	}, run)
}

func (svc *Service) InitStart(ctx context.Context, kvc kvstore.Client, configs structs.ConfigsMap, args map[string]interface{}) error {
	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusInitServiceStarting, statusInitServiceStarted, statusInitServiceStartFailed)

	val, err := sl.Load()
	if err == nil {
		if val > statusInitServiceStartFailed {
			return svc.Start(ctx, configs.Commands())
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
	})
}

func (svc *Service) initStart(ctx context.Context, kvc kvstore.Client, configs structs.ConfigsMap, args map[string]interface{}) error {

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	// TODO:remove later,conater will start while updateConfigs
	for i := range units {
		err := units[i].startContainer(ctx)
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

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
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

	if kvc != nil {
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

func (svc *Service) Start(ctx context.Context, cmds structs.Commands) error {

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

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceStarting, statusServiceStarted, statusServiceStartFailed)

	return sl.Run(isnotInProgress, start)
}

func (svc *Service) UpdateUnitsConfigs(ctx context.Context, configs structs.ConfigsMap, args map[string]interface{}) (err error) {

	update := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		return svc.updateConfigs(ctx, units, configs, args)
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceConfigUpdating, statusServiceConfigUpdated, statusServiceConfigUpdateFailed)

	return sl.Run(isnotInProgress, update)
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

func (svc *Service) UpdateConfig(ctx context.Context, nameOrID string, args map[string]string) error {
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

func (svc *Service) Stop(ctx context.Context, containers bool) error {

	stop := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		return svc.stop(ctx, units, containers)
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceStoping, statusServiceStoped, statusServiceStopFailed)

	return sl.Run(isnotInProgress, stop)
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

func (svc *Service) Exec(ctx context.Context, nameOrID string, cmd []string, detach bool) (types.ContainerExecInspect, error) {
	u, err := svc.getUnit(nameOrID)
	if err != nil {
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
		if task != nil {
			return svc.so.SetTask(*task)
		}

		return nil
	})

	return sl.Run(isnotInProgress, remove)
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

func (svc *Service) removeContainers(ctx context.Context, units []*unit, force, rmVolumes bool) error {

	for _, u := range units {
		engine := u.getEngine()
		if engine == nil {
			return nil
		}

		client := engine.ContainerAPIClient()
		if client == nil {
			return nil
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

func (svc *Service) deleteCondition() error {
	return nil
}

func (svc *Service) deregisterSerivces(ctx context.Context, reg kvstore.Register, units []*unit) error {
	for i := range units {
		err := reg.DeregisterService(ctx, "units", units[i].u.ID, "", "")
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) Image() (database.Image, error) {
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

func (svc *Service) generateUnitConfig(ctx context.Context, nameOrID string, args map[string]string) (structs.ConfigCmds, error) {
	return structs.ConfigCmds{}, nil
}

func (svc *Service) generateUnitsCmd(ctx context.Context) (structs.Commands, error) {
	return svc.pc.GetCommands(ctx, svc.svc.ID)
}
