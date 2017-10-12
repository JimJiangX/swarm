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
	unit        unit
	engine      *cluster.Engine
	container   *cluster.Container
	volumes     []database.Volume
	networkings []database.IP
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
				return errors.Errorf("unit %s isnot belongs to Service %s", nameOrID, svc.svc.Name)
			}

			lvs, err := got.uo.ListVolumesByUnitID(got.u.ID)
			if err != nil {
				return err
			}
			ips, err := got.uo.ListIPByUnitID(got.u.ID)
			if err != nil {
				return err
			}

			old = baseContainer{
				unit:        *got,
				engine:      got.getEngine(),
				container:   got.getContainer(),
				volumes:     lvs,
				networkings: ips,
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
			adds, pendings, err := gd.scaleAllocation(ctx, svc, actor, false, false,
				len(units)+1, candidates, nil)
			defer func() {
				if err != nil {
					_err := svc.removeUnits(ctx, adds, nil)
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

			defer func() {
				if err != nil {
					_err := svc.so.SetIPs(old.networkings)
					if _err != nil {
						err = errors.Errorf("%+v\nmgirate networkings:%+v", err, _err)
					}
				}
			}()

			// migrate networkings
			news.networkings, err = actor.AllocDevice(news.unit.u.EngineID, news.unit.u.ID, old.networkings)
			if err != nil {
				return err
			}

			// migrate volumes
			news.volumes, err = actor.MigrateVolumes(news.unit.u.ID, old.engine, news.engine, old.volumes)
			if err != nil {
				return err
			}

			defer func() {
				if err != nil {
					// migrate volumes
					_, _err := actor.MigrateVolumes(old.unit.u.ID, news.engine, old.engine, news.volumes)
					if _err != nil {
						err = errors.Errorf("%+v\nmgirate volumes:%+v", err, _err)
					}
				}
			}()

			auth, err := gd.AuthConfig()
			if err != nil {
				return err
			}

			err = svc.runContainer(ctx, pendings, false, auth)
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

			err = svc.start(ctx, adds, cms.Commands())
			if err != nil {
				return err
			}
		}
		{
			// clean old
			err := svc.deregisterServices(ctx, gd.KVClient(), []*unit{&old.unit})
			if err != nil {
				return err
			}

			err = svc.removeContainers(ctx, []*unit{&old.unit}, false, true)
			if err != nil {
				return err
			}

			err = renameContainer(&news.unit, old.unit.u.Name)
			if err != nil {
				return err
			}

			err = svc.so.MigrateUnit(news.unit.u.ID, old.unit.u.ID, old.unit.u.Name)
			if err != nil {
				return err
			}

			cms, err := svc.generateUnitsConfigs(ctx, nil)
			if err != nil {
				return err
			}

			err = svc.Compose(ctx, gd.pluginClient)
			if err != nil {
				return err
			}

			u, err := svc.getUnit(old.unit.u.ID)
			if err != nil {
				return err
			}

			err = registerUnits(ctx, []*unit{u}, gd.KVClient(), cms)
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

func migrateNetworking(orm database.NetworkingOrmer, src, new []database.IP) ([]database.IP, error) {
	if len(src) != len(new) {
		return nil, errors.New("invalid input")
	}

	dst := make([]database.IP, len(src))
	copy(dst, src)

	for i := range dst {
		dst[i].Bandwidth = new[i].Bandwidth
		dst[i].Bond = new[i].Bond
		dst[i].Engine = new[i].Engine
		dst[i].UnitID = new[i].UnitID
		// reset IP
		new[i].UnitID = ""
		new[i].Engine = ""
		new[i].Bandwidth = 0
		new[i].Bond = ""
	}

	set := make([]database.IP, len(dst)+len(new))
	n := copy(set, dst)
	copy(set[n:], new)

	err := orm.SetIPs(set)

	return dst, err
}
