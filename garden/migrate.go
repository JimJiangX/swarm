package garden

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type baseContainer struct {
	unit      unit
	engine    *cluster.Engine
	container *cluster.Container
	volumes   []database.Volume
}

// ServiceMigrate migrate an unit to other hosts,include volumes、networkings,clean the old container.
func (gd *Garden) ServiceMigrate(ctx context.Context, svc *Service, nameOrID string, candidates []string, async bool) (string, error) {
	migrate := func() error {
		var old, news baseContainer

		units, err := svc.getUnits()
		if err != nil {
			return err
		}
		{
			// the assigned unit being migrating
			got := getUnit(units, nameOrID)
			if got == nil {
				return errors.Errorf("unit %s is not exist in service %s", nameOrID, svc.svc.Name)
			}

			lvs, err := svc.so.ListVolumesByUnitID(got.u.ID)
			if err != nil {
				return err
			}

			old = baseContainer{
				unit:      *got,
				engine:    got.getEngine(),
				container: got.getContainer(),
				volumes:   lvs,
			}
			if old.engine == nil {
				old.engine = &cluster.Engine{
					ID: old.unit.u.EngineID,
				}
			}

			if old.container == nil {
				c, err := getContainerFromKV(ctx, gd.kvClient, old.unit.u.ContainerID)
				if err != nil {
					return err
				}
				old.container = c
			}
		}

		{
			actor := alloc.NewAllocator(gd.ormer, gd.Cluster)
			adds, pendings, err := gd.scaleAllocation(ctx, svc, actor, true,
				len(units)+1, candidates, nil, nil)
			defer func() {
				if err != nil {
					_err := svc.removeUnits(ctx, units, gd.kvClient)
					if _err != nil {
						err = errors.Errorf("%+v\nremove new addition units:%+v", err, _err)
					}
				}
			}()
			if err != nil {
				return err
			}

			if len(adds) > 0 {
				news.unit = *adds[0]
			}
			// migrate volumes
			out, err := actor.MigrateVolumes(news.unit.u.ID, old.engine, news.engine, old.volumes)
			news.volumes = out
			if err != nil {
				return err
			}

			auth, err := gd.AuthConfig()
			if err != nil {
				return err
			}

			err = svc.runContainer(ctx, pendings, auth)
			if err != nil {
				return err
			}

			if len(pendings) > 0 {
				news.unit.u = pendings[0].Unit
				news.container = news.unit.getContainer()
				news.engine = news.unit.getEngine()
			}

			cms, err := svc.generateUnitsConfigs(ctx, nil)
			if err != nil {
				return err
			}

			err = svc.start(ctx, adds, nil, cms.Commands())
			if err != nil {
				return err
			}

			err = updateUnitRegister(ctx, gd.kvClient, old.unit, news.unit, cms)
			if err != nil {
				return err
			}
		}
		{
			// clean old
			err := svc.removeContainers(ctx, []*unit{&old.unit}, false, true)
			if err != nil {
				return err
			}

			err = svc.Compose(ctx, gd.pluginClient)
			if err != nil {
				return err
			}
		}

		return nil
	}

	task := database.NewTask(svc.svc.Name, database.UnitMigrateTask, svc.svc.ID, nameOrID, nil, 300)

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &task,
		statusServiceUnitMigrating, statusServiceUnitMigrated, statusServiceUnitMigrateFailed)

	err := sl.Run(isnotInProgress, migrate, async)

	return task.ID, err
}

// deregister old service and register new
func updateUnitRegister(ctx context.Context, kvc kvstore.Client, old, new unit, cmds structs.ConfigsMap) error {
	// deregister service
	err := deregisterService(ctx, kvc, "units", old.u.ID)
	if err != nil {
		return err
	}

	return registerUnits(ctx, []*unit{&new}, kvc, cmds)
}
