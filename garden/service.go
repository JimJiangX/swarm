package garden

import (
	"encoding/json"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var containerKV = "swarm/containers/"

type Service struct {
	sl           statusLock
	so           database.ServiceOrmer
	spec         structs.ServiceSpec
	cluster      cluster.Cluster
	pluginClient pluginapi.PluginAPI
	options      scheduleOption

	imageName    string
	imageVersion string
}

func newService(spec structs.ServiceSpec, so database.ServiceOrmer, cluster cluster.Cluster, pc pluginapi.PluginAPI) *Service {
	image, version := (database.Service)(spec.Service).ParseImage()

	return &Service{
		spec:         spec,
		so:           so,
		cluster:      cluster,
		pluginClient: pc,
		sl:           newStatusLock(spec.ID, so),
		imageName:    image,
		imageVersion: version,
	}
}

func (gd *Garden) NewService(spec structs.ServiceSpec) *Service {
	return newService(spec, gd.ormer, gd.Cluster, gd.pluginClient)
}

func (svc *Service) getUnit(nameOrID string) (*unit, error) {
	u, err := svc.so.GetUnit(nameOrID)
	if err != nil {
		return nil, err
	}

	if u.ServiceID != svc.spec.ID {
		return nil, nil
	}

	return newUnit(u, svc.so, svc.cluster), nil
}

func (svc *Service) getUnits() ([]*unit, error) {
	list, err := svc.so.ListUnitByServiceID(svc.spec.ID)
	if err != nil {
		return nil, err
	}

	units := make([]*unit, 0, len(list))

	for i := range list {
		units[i] = newUnit(list[i], svc.so, svc.cluster)
	}

	return units, nil
}

func (svc Service) Spec() structs.ServiceSpec {
	return svc.spec
}

func (svc *Service) CreateContainer(pendings []pendingUnit, authConfig *types.AuthConfig) (err error) {
	defer func() {
		ids := make([]string, len(pendings))
		for i := range pendings {
			ids[i] = pendings[i].swarmID
		}
		svc.cluster.RemovePendingContainer(ids...)
	}()

	ok, val, err := svc.sl.CAS(statusServiceContainerCreating, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceContainerCreating, val), "Service create containers")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		status := statusServiceContainerCreated
		if err != nil {
			status = statusServiceContainerCreateFailed
		}

		_err := svc.sl.SetStatus(status)
		if _err != nil {
			logrus.WithField("Service", svc.spec.Name).Errorf("orm:Set Service status:%d,%+v", status, _err)
		}
	}()

	for _, pu := range pendings {
		eng := svc.cluster.Engine(pu.Unit.EngineID)
		if eng == nil {
			return nil
		}

		for i := range pu.volumes {
			v, err := eng.CreateVolume(&pu.volumes[i])
			if err != nil {
				return err
			}
			pu.config.HostConfig.Binds = append(pu.config.HostConfig.Binds, v.Name)
		}
		// TODO: Create Network

		c, err := eng.CreateContainer(pu.config, pu.Unit.Name, true, authConfig)
		if err != nil {
			return err
		}
		pu.Unit.ContainerID = c.ID
	}

	return nil
}

func (svc *Service) InitStart(ctx context.Context, kvc kvstore.Client, configs structs.ConfigsMap) (err error) {
	ok, val, err := svc.sl.CAS(statusServiceStarting, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceStarting, val), "Service init start")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		status := statusServiceStarted
		if err != nil {
			status = statusServiceStartFailed
		}

		_err := svc.sl.SetStatus(status)
		if _err != nil {
			logrus.WithField("Service", svc.spec.Name).Errorf("orm:Set Service status:%d,%+v", status, _err)
		}
	}()

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	if configs == nil {
		configs, err = svc.generateUnitsConfigs(ctx, nil)
		if err != nil {
			return err
		}
	}

	// start containers and update configs
	err = svc.updateConfigs(ctx, units, configs)
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

			c := u.getContainer()

			val, err := json.Marshal(c)
			if err != nil {
				return errors.Wrapf(err, "JSON marshal Container %s", u.u.Name)
			}

			err = kvc.PutKV(containerKV+c.ID, val)
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

func (svc *Service) Start(ctx context.Context, cmds structs.Commands) (err error) {
	ok, val, err := svc.sl.CAS(statusServiceStarting, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceStarting, val), "Service start")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		status := statusServiceStarted
		if err != nil {
			status = statusServiceStartFailed
		}

		_err := svc.sl.SetStatus(status)
		if _err != nil {
			logrus.WithField("Service", svc.spec.Name).Errorf("orm:Set Service status:%d,%+v", status, _err)
		}
	}()

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

	// get init cmd
	for i := range units {
		cmd := cmds.GetCmd(units[i].u.ID, structs.StartServiceCmd)

		_, err = units[i].containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) UpdateUnitsConfigs(ctx context.Context, configs structs.ConfigsMap) (err error) {
	ok, val, err := svc.sl.CAS(statusServiceConfigUpdating, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceConfigUpdating, val), "Service update units configs")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		status := statusServiceConfigUpdated
		if err != nil {
			status = statusServiceConfigUpdateFailed
		}

		_err := svc.sl.SetStatus(status)
		if _err != nil {
			logrus.WithField("Service", svc.spec.Name).Errorf("orm:Set Service status:%d,%+v", status, _err)
		}
	}()

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	return svc.updateConfigs(ctx, units, configs)
}

// updateConfigs update units configurationFile,
// generate units configs if configs is nil,
// start units containers before update container configurationFile.
func (svc *Service) updateConfigs(ctx context.Context, units []*unit, configs structs.ConfigsMap) (err error) {
	if configs == nil {
		configs, err = svc.generateUnitsConfigs(ctx, nil)
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

		err := units[i].updateServiceConfig(ctx, config.Mount, config.Content)
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

	err = u.updateServiceConfig(ctx, config.Mount, config.Content)

	return err
}

func (svc *Service) Stop(ctx context.Context) (err error) {
	ok, val, err := svc.sl.CAS(statusServiceStoping, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceStoping, val), "Service stop")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		status := statusServiceStoped
		if err != nil {
			status = statusServiceStopFailed
		}

		_err := svc.sl.SetStatus(status)
		if _err != nil {
			logrus.WithField("Service", svc.spec.Name).Errorf("orm:Set Service status:%d,%+v", status, _err)
		}
	}()

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	return svc.stop(ctx, units)
}

func (svc *Service) stop(ctx context.Context, units []*unit) error {
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

	for i := range units {
		err = units[i].stopContainer(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) Update(updateConfig *container.UpdateConfig) error {
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
	ok, val, err := svc.sl.CAS(statusServiceDeleting, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceDeleting, val), "Service delete")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}
		if err != nil {
			_err := svc.sl.SetStatus(statusServiceDeleteFailed)
			if _err != nil {
				logrus.WithField("Service", svc.spec.Name).Errorf("orm:Set Service status:%d,%+v", statusServiceDeleteFailed, _err)
			}
		}
	}()

	err = svc.deleteCondition()
	if err != nil {
		return err
	}

	units, err := svc.getUnits()
	if err != nil {
		return err
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

	err = svc.so.DelServiceRelation(svc.spec.ID, true)

	return err
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

func (svc *Service) deregisterSerivces(ctx context.Context, r kvstore.Register, units []*unit) error {
	for i := range units {

		host, err := units[i].getHostIP()
		if err != nil {
			return err
		}

		err = r.DeregisterService(ctx, host, units[i].u.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) Image() (database.Image, error) {

	return svc.so.GetImage(svc.spec.Image)
}

func (svc *Service) Requires(ctx context.Context) (structs.RequireResource, error) {

	return svc.pluginClient.GetImageRequirement(ctx, svc.imageName, svc.imageVersion)
}

func (svc *Service) generateUnitsConfigs(ctx context.Context, args map[string]interface{}) (structs.ConfigsMap, error) {

	return svc.pluginClient.GenerateServiceConfig(ctx, svc.spec)
}

func (svc *Service) GenerateUnitsConfigs(ctx context.Context, args map[string]interface{}) (structs.ConfigsMap, error) {
	return svc.generateUnitsConfigs(ctx, args)
}

func (svc *Service) generateUnitConfig(ctx context.Context, nameOrID string, args map[string]string) (structs.ConfigCmds, error) {
	return structs.ConfigCmds{}, nil
}

func (svc *Service) generateUnitsCmd(ctx context.Context) (structs.Commands, error) {
	return svc.pluginClient.GetCommands(ctx, svc.spec.ID)
}
