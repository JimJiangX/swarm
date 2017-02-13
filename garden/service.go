package garden

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
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
	image, version := spec.ParseImage()

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

func (svc *Service) CreateContainer(pendings []pendingUnit, authConfig *types.AuthConfig) error {
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
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		status := statusServiceContainerCreated
		if err != nil {
			status = statusServiceContainerCreateFailed
		}

		err := svc.sl.SetStatus(status)
		if err != nil {

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

func (svc *Service) InitStart(ctx context.Context, kvc kvstore.Client, configs structs.ConfigsMap) error {
	ok, val, err := svc.sl.CAS(statusServiceStarting, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		status := statusServiceStarted
		if err != nil {
			status = statusServiceStartFailed
		}

		err := svc.sl.SetStatus(status)
		if err != nil {

		}
	}()

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	for i := range units {
		err := units[i].startContainer(ctx)
		if err != nil {
			return err
		}
	}

	if configs == nil {
		configs, err = svc.generateUnitsConfigs(ctx, nil)
		if err != nil {
			return err
		}
	}

	for i := range units {
		config, ok := configs.Get(units[i].u.ID)
		if !ok {
			// TODO: return nil
		}

		err := units[i].updateServiceConfig(ctx, config.Mount, config.Content)
		if err != nil {
			return err
		}

		cmd := config.GetCmd(structs.InitServiceCmd)

		_, err = units[i].containerExec(ctx, cmd)
		if err != nil {
			return err
		}
	}

	if kvc != nil {
		// register to kv store and third-part services
		for i := range units {
			host := ""
			c := units[i].getContainer()
			if c != nil {
				host = c.Engine.IP
			}
			if host == "" {
				// TODO: get node IP
			}
			val, err := json.Marshal(c)
			if err != nil {
				// TODO: json marshal error
			}

			err = kvc.PutKV(containerKV+c.ID, val)
			if err != nil {
				return err
			}

			config, ok := configs.Get(units[i].u.ID)
			if !ok {
				// TODO:
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

func (svc *Service) Start(ctx context.Context) error {
	ok, val, err := svc.sl.CAS(statusServiceStarting, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		status := statusServiceStarted
		if err != nil {
			status = statusServiceStartFailed
		}

		err := svc.sl.SetStatus(status)
		if err != nil {

		}
	}()

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	for i := range units {
		err = units[i].startContainer(ctx)
		if err != nil {
			return err
		}
	}

	cmds, err := svc.generateUnitsCmd(ctx)
	if err != nil {
		return err
	}

	// get init cmd
	for i := range units {
		cmd := cmds.GetCmd(units[i].u.ID, structs.StartServiceCmd)

		_, err = units[i].containerExec(ctx, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) Stop(ctx context.Context) error {
	ok, val, err := svc.sl.CAS(statusServiceStoping, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		status := statusServiceStoped
		if err != nil {
			status = statusServiceStopFailed
		}

		err := svc.sl.SetStatus(status)
		if err != nil {

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

		_, err = units[i].containerExec(ctx, cmd)
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

func (svc *Service) Exec(ctx context.Context, nameOrID string, cmd []string) (types.ContainerExecInspect, error) {
	u, err := svc.getUnit(nameOrID)
	if err != nil {
		return types.ContainerExecInspect{}, err
	}

	return u.containerExec(ctx, cmd)
}

func (svc *Service) Remove(ctx context.Context, r kvstore.Register) error {
	ok, val, err := svc.sl.CAS(statusServiceDeleting, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		if err != nil {
			err := svc.sl.SetStatus(statusServiceDeleteFailed)
			if err != nil {
				// TODO:logrus
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

	// TODO:remove data from database
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

		timeout := 30 * time.Second
		err := client.ContainerStop(ctx, u.u.Name, &timeout)
		engine.CheckConnectionErr(err)
		if err != nil {
			return err
		}

		options := types.ContainerRemoveOptions{
			RemoveVolumes: rmVolumes,
			RemoveLinks:   false,
			Force:         force,
		}
		err = client.ContainerRemove(ctx, u.u.Name, options)
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
		host := ""
		if e := units[i].getEngine(); e != nil {
			host = e.IP
		}
		if host == "" {
			// TODO: get node IP
		}
		err := r.DeregisterService(ctx, host, units[i].u.ID)
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
