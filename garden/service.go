package garden

import (
	"encoding/json"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	consulapi "github.com/hashicorp/consul/api"
	"golang.org/x/net/context"
)

var containerKV = "swarm/containers/"

type Service struct {
	sl      statusLock
	svc     database.Service
	so      database.ServiceOrmer
	cluster cluster.Cluster
	units   []database.Unit
}

func newService(svc database.Service, so database.ServiceOrmer, cluster cluster.Cluster) *Service {
	return &Service{
		svc:     svc,
		so:      so,
		cluster: cluster,
		sl:      newStatusLock(svc.ID, so),
	}
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

	svc.svc.Status = val

	if !ok {
		return err
	}

	defer func() {
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

	svc.svc.Status = val

	if !ok {
		return err
	}

	defer func() {
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

		err := units[i].updateServiceConfig(ctx, config.Path, config.Context)
		if err != nil {
			return err
		}

		cmd := config.GetCmd(structs.InitServiceCmd)

		_, err = units[i].containerExec(ctx, cmd)
		if err != nil {
			return err
		}
	}

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

		r := config.GetServiceRegistration(units[i].u.ID)
		_consul := consulapi.AgentServiceRegistration(r.Consul)

		err = kvc.RegisterService(ctx, host, _consul, r.Horus)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) Start(ctx context.Context) error {
	ok, val, err := svc.sl.CAS(statusServiceStarting, isInProgress)
	if err != nil {
		return err
	}

	svc.svc.Status = val

	if !ok {
		return err
	}

	defer func() {
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

	svc.svc.Status = val

	if !ok {
		return err
	}

	defer func() {
		status := statusServiceStoped
		if err != nil {
			status = statusServiceStopFailed
		}

		err := svc.sl.SetStatus(status)
		if err != nil {

		}
	}()

	return svc.stop(ctx)
}

func (svc *Service) stop(ctx context.Context) error {
	units, err := svc.getUnits()
	if err != nil {
		return err
	}

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

func (svc *Service) Scale() error {
	return nil
}

func (svc *Service) Update() error {
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

	err = u.updateServiceConfig(ctx, config.Path, config.Context)

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

	svc.svc.Status = val

	if !ok {
		return err
	}

	defer func() {
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

	err = svc.deregisterSerivces(ctx, r)
	if err != nil {
		return err
	}

	err = svc.removeContainers(ctx, true, false)
	if err != nil {
		return err
	}

	err = svc.removeVolumes(ctx)
	if err != nil {
		return err
	}

	// TODO:remove data from database
	err = svc.so.DelServiceRelation(svc.svc.ID, true)

	return err
}

func (svc *Service) removeContainers(ctx context.Context, force, rmVolumes bool) error {
	units, err := svc.getUnits()
	if err != nil {
		return err
	}

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

func (svc *Service) removeVolumes(ctx context.Context) error {
	units, err := svc.getUnits()
	if err != nil {
		return err
	}

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

func (svc *Service) deregisterSerivces(ctx context.Context, r kvstore.Register) error {
	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	for i := range units {
		host := ""
		if e := units[i].getEngine(); e != nil {
			host = e.IP
		}
		if host == "" {
			// TODO: get node IP
		}
		err = r.DeregisterService(ctx, host, units[i].u.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (svc *Service) Image() (database.Image, error) {
	return database.Image{}, nil
}

func (svc *Service) Requires(ctx context.Context) (structs.RequireResource, error) {
	// imageName imageVersion

	return structs.RequireResource{}, nil
}

func (svc *Service) generateUnitsConfigs(ctx context.Context, args map[string]string) (structs.ConfigsMap, error) {
	return structs.ConfigsMap{}, nil
}

func (svc *Service) generateUnitConfig(ctx context.Context, nameOrID string, args map[string]string) (structs.ConfigCmds, error) {
	return structs.ConfigCmds{}, nil
}

func (svc *Service) generateUnitsCmd(ctx context.Context) (structs.Commands, error) {
	return structs.Commands{}, nil
}
