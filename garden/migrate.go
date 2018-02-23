package garden

import (
	"strings"
	"time"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/vars"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// ServiceMigrate migrate an unit to other hosts,include volumes、networkings,clean the old container.
func (gd *Garden) ServiceMigrate(ctx context.Context, svc *Service, req structs.PostUnitMigrate, async bool) (string, error) {

	task := database.NewTask(svc.Name(), database.UnitMigrateTask, svc.ID(), req.NameOrID, nil, 300)

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, &task,
		statusServiceUnitMigrating, statusServiceUnitMigrated, statusServiceUnitMigrateFailed)

	err := sl.Run(isnotInProgress, func() error {
		return gd.rebuildUnit(ctx, svc, req.NameOrID, req.Candidates, true, req.Compose)
	}, async)

	return task.ID, err
}

type baseContainer struct {
	unit        unit
	engine      *cluster.Engine
	container   *cluster.Container
	volumes     []database.Volume
	networkings []database.IP
}

func getUnitBaseContainer(ctx context.Context, svc *Service, unit string, kvc kvstore.Client) (baseContainer, error) {
	got, err := svc.getUnit(unit)
	if err != nil || got == nil {
		return baseContainer{}, errors.Errorf("unit %s isnot belongs to Service %s,%+v", unit, svc.Name(), err)
	}

	lvs, err := got.uo.ListVolumesByUnitID(got.u.ID)
	if err != nil {
		return baseContainer{}, err
	}

	ips, err := got.uo.ListIPByUnitID(got.u.ID)
	if err != nil {
		return baseContainer{}, err
	}

	base := baseContainer{
		unit:        *got,
		engine:      got.getEngine(),
		container:   got.getContainer(),
		volumes:     lvs,
		networkings: ips,
	}

	if base.engine == nil {
		base.engine = &cluster.Engine{
			ID: base.unit.u.EngineID,
		}
	}

	if base.container == nil {
		c, err := getContainerFromKV(ctx, kvc, base.unit.u.ContainerID)
		if err != nil {
			return base, err
		}

		base.container = c
	}

	return base, err
}

func (gd *Garden) rebuildUnit(ctx context.Context, svc *Service, nameOrID string, candidates []string, migrate, compose bool) error {
	var news baseContainer
	var cms structs.ConfigsMap

	// 1.the assigned unit being migrating
	old, err := getUnitBaseContainer(ctx, svc, nameOrID, gd.kvClient)
	if err != nil {
		return err
	}

	{
		// 2.generate new database.Unit,insert into database
		add, err := svc.addNewUnit(1)
		if err != nil {
			return err
		}

		// required alloc new volume
		vr := true
		if migrate {
			vr = false
		}

		// 3.alloc new node for new unit
		actor := alloc.NewAllocator(gd.ormer, gd.Cluster)
		adds, pendings, err := gd.scaleAllocation(ctx, svc, nameOrID, actor, vr, false,
			add, candidates, nil)

		defer func() {
			if err != nil {
				_err := svc.removeUnits(ctx, adds, nil)
				if _err != nil {
					err = errors.Errorf("%+v\nremove new addition units:%+v", err, _err)
				}

				_err = svc.so.SetIPs(old.networkings)
				if _err != nil {
					err = errors.Errorf("%+v\nmgirate networkings:%+v", err, _err)
				}
			}
		}()
		if err != nil {
			return err
		}

		if len(pendings) == 0 {
			return errors.New("scaleAlloc:allocation failed")
		} else {
			news.unit = *adds[0]
			news.unit.u = pendings[0].Unit
			news.engine = news.unit.getEngine()
		}

		// 4.migrate IP for new unit
		{
			// migrate networkings
			news.networkings, err = actor.AllocDevice(news.unit.u.EngineID, news.unit.u.ID, old.networkings)
			if err != nil {
				return err
			}
			if len(pendings) > 0 {
				for i := range news.networkings {
					ip := utils.Uint32ToIP(news.networkings[i].IPAddr)
					if ip == nil {
						continue
					}

					pendings[0].config.Config.Env = append(pendings[0].config.Config.Env, "IPADDR="+ip.String())
					pendings[0].config.Config.Env = append(pendings[0].config.Config.Env, "NET_DEV="+news.networkings[i].Bond)
					break
				}
			}
		}

		// 5.migrate volume for new unit
		if migrate {
			// stop container before migrate volume
			err = old.unit.stopContainer(ctx)
			if err != nil {
				return err
			}
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

		// 6.run new unit container
		auth, err := gd.AuthConfig()
		if err != nil {
			return err
		}

		err = svc.runContainer(ctx, pendings, false, auth)
		if err != nil {
			return err
		}

		// 7.start new unit service
		{
			// using old unit config as news unit
			cc, _err := svc.getUnitConfig(ctx, old.unit.u.ID)
			if _err != nil {
				return _err
			}

			if cc.Registration.Horus != nil {
				node, _err := svc.so.GetNode(news.unit.u.EngineID)
				if _err != nil {
					return _err
				}
				cc.Registration.Horus.Service.Container.HostName = node.ID
			}

			cms = make(structs.ConfigsMap)
			cms[news.unit.u.ID] = cc
			cms[old.unit.u.ID] = cc

			// start unit service
			if migrate {
				err = svc.start(ctx, adds, cms.Commands())
			} else {
				err = svc.initStart(ctx, adds, nil, cms, nil)
			}
			if err != nil {
				return err
			}

			for i := range adds {
				cmd := cms.GetCmd(adds[i].u.ID, structs.MigrateRebuildCmd)
				if len(cmd) == 0 {
					continue
				}

				cmd = append(cmd, vars.Root.Role, vars.Root.User, vars.Root.Password)
				_, err := adds[i].ContainerExec(ctx, cmd, false, nil)
				if err != nil {
					return err
				}
			}
		}
	}

	// 8.remove old unit container & volume
	{
		healthy := old.engine.IsHealthy()
		rmUnits := []*unit{&old.unit}

		err := svc.removeContainers(ctx, rmUnits, false, true)
		if err != nil && healthy {
			return err
		}

		if !migrate {
			err = svc.removeVolumes(ctx, rmUnits)
			if err != nil && healthy {
				return err
			}
		}

		// rename new container name as old unit name
		err = renameContainer(&news.unit, old.unit.u.Name)
		if err != nil {
			return err
		}

		// waiting for update rename container event done
		time.Sleep(10 * time.Second)

		// 9.migrate database related old unit
		err = svc.so.MigrateUnit(news.unit.u.ID, old.unit.u.ID, old.unit.u.Name)
		if err != nil {
			return err
		}
	}

	// 10.register to third-part system
	{
		u, err := svc.getUnit(old.unit.u.ID)
		if err != nil {
			return err
		}

		c := gd.Container(u.u.ContainerID)
		err = saveContainerToKV(ctx, gd.KVClient(), c)

		if old.engine.ID != u.u.EngineID {
			err = registerUnits(ctx, []*unit{u}, gd.KVClient(), cms)
		}
		if err != nil {
			return err
		}
	}

	// 11.compose
	if compose {
		err := svc.Compose(ctx, gd.pluginClient)
		if err != nil {
			return err
		}
	}

	return nil
}
