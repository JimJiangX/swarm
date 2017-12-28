package garden

import (
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
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
func (gd *Garden) ServiceMigrate(ctx context.Context, svc *Service, req structs.PostUnitMigrate, async bool) (string, error) {

	task := database.NewTask(svc.Name(), database.UnitMigrateTask, svc.ID(), req.NameOrID, nil, 300)

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, &task,
		statusServiceUnitMigrating, statusServiceUnitMigrated, statusServiceUnitMigrateFailed)

	err := sl.Run(isnotInProgress, func() error {
		return gd.rebuildUnit(ctx, svc, req.NameOrID, req.Candidates, true)
	}, async)

	return task.ID, err
}

func (gd *Garden) rebuildUnit(ctx context.Context, svc *Service, nameOrID string, candidates []string, migrate bool) error {
	var old, news baseContainer
	var cms structs.ConfigsMap

	// the assigned unit being migrating
	got, err := svc.getUnit(nameOrID)
	if err != nil || got == nil {
		return errors.Errorf("unit %s isnot belongs to Service %s,%+v", nameOrID, svc.Name(), err)
	}

	add, err := svc.addNewUnit(1)
	if err != nil {
		return err
	}
	news.unit = *(newUnit(add[0], svc.so, svc.cluster))

	{

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
		// required alloc new volume
		vr := true
		if migrate {
			vr = false
		}

		actor := alloc.NewAllocator(gd.ormer, gd.Cluster)
		adds, pendings, err := gd.scaleAllocation(ctx, svc, actor, vr, false,
			add, candidates, nil)
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

		defer func() {
			if err != nil {
				_err := svc.so.SetIPs(old.networkings)
				if _err != nil {
					err = errors.Errorf("%+v\nmgirate networkings:%+v", err, _err)
				}
			}
		}()

		{
			// migrate networkings
			news.networkings, err = actor.AllocDevice(news.unit.u.EngineID, news.unit.u.ID, old.networkings)
			if err != nil {
				return err
			}
			if len(pendings) > 0 {
				for i := range news.networkings {
					ip := utils.Uint32ToIP(news.networkings[i].IPAddr)
					pendings[0].config.Config.Env = append(pendings[0].config.Config.Env, "IPADDR="+ip.String())
				}
			}
		}

		if migrate {
			// migrate volumes
			news.volumes, err = actor.MigrateVolumes(news.unit.u.ID, old.engine, news.engine, old.volumes)
			if err != nil {
				return err
			}

			if len(pendings) > 0 {
				pendings[0].config.HostConfig.Binds = old.container.Config.HostConfig.Binds

				for _, env := range old.container.Config.Env {

					if strings.Contains(env, "_DIR=/DBAAS") {
						pendings[0].config.Env = append(pendings[0].config.Env, env)
					}
				}
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
		}

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

		{
			// using old unit config as news unit
			cc, err := svc.getUnitConfig(ctx, old.unit.u.ID)
			if err != nil {
				return err
			}

			if cc.Registration.Horus != nil {
				node, err := svc.so.GetNode(news.unit.u.EngineID)
				if err != nil {
					return err
				}
				cc.Registration.Horus.Service.Container.HostName = node.ID
			}

			cms = make(structs.ConfigsMap)
			cms[news.unit.u.ID] = cc
			cms[old.unit.u.ID] = cc
		}

		// start unit service
		if migrate {
			err = svc.start(ctx, adds, cms.Commands())
		} else {
			err = svc.initStart(ctx, adds, nil, cms, nil)
		}
		if err != nil {
			return err
		}
	}
	{
		// remove old unit container & volume
		rmUnits := []*unit{&old.unit}

		err = svc.removeContainers(ctx, rmUnits, false, true)
		if err != nil {
			return err
		}

		if !migrate {
			err = svc.removeVolumes(ctx, rmUnits)
			if err != nil {
				return err
			}
		}
	}
	{
		// rename new container name as old unit name
		err = renameContainer(&news.unit, old.unit.u.Name)
		if err != nil {
			return err
		}

		err = svc.so.MigrateUnit(news.unit.u.ID, old.unit.u.ID, old.unit.u.Name)
		if err != nil {
			return err
		}
	}
	{
		u, err := svc.getUnit(old.unit.u.ID)
		if err != nil {
			return err
		}

		if old.engine.ID == news.engine.ID {
			c := gd.Container(news.unit.u.ContainerID)
			err = saveContainerToKV(ctx, gd.KVClient(), c)
		} else {
			err = registerUnits(ctx, []*unit{u}, gd.KVClient(), cms)
		}
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
