package garden

import (
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"golang.org/x/net/context"
)

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

func (svc *Service) InitStart(ctx context.Context) error {
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

	// get init cmd

	cmd := []string{}
	for i := range units {
		_, err := units[i].containerExec(ctx, cmd)
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
		err := units[i].startContainer(ctx)
		if err != nil {
			return err
		}
	}

	// get init cmd
	cmd := []string{}
	for i := range units {
		_, err := units[i].containerExec(ctx, cmd)
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

	// get init cmd
	cmd := []string{}
	for i := range units {
		_, err := units[i].containerExec(ctx, cmd)
		if err != nil {
			return err
		}
	}

	for i := range units {
		err := units[i].stopContainer(ctx)
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

func (svc *Service) UpdateConfig() error {
	return nil
}

func (svc *Service) Exec(ctx context.Context, nameOrID string, cmd []string) (types.ContainerExecInspect, error) {
	u, err := svc.getUnit(nameOrID)
	if err != nil {
		return types.ContainerExecInspect{}, err
	}

	return u.containerExec(ctx, cmd)
}

func (svc *Service) Remove(ctx context.Context) error {
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

			}
		}
	}()

	err = svc.deleteCondition()
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

	return nil
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
