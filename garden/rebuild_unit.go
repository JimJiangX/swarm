package garden

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// RebuildUnits rebuild the assigned units on candidates host,
// remove original containers,than startup service
func (gd *Garden) RebuildUnits(ctx context.Context, actor alloc.Allocator, svc *Service,
	req structs.UnitRebuildRequest, async bool) (string, error) {
	if len(req.Units) < 1 {
		return "", nil
	}

	rebuild := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		rm := make([]*unit, 0, len(req.Units))
		for _, id := range req.Units {
			u := getUnit(units, id)
			if u == nil {
				return errors.Errorf("unit '%s' isnot belong to Service '%s'", id, svc.svc.Name)
			}
			rm = append(rm, u)
		}

		networkings := make([][]database.IP, 0, len(rm))
		for i := range rm {
			out, err := rm[i].uo.ListIPByUnitID(rm[i].u.ID)
			if err != nil {
				return err
			}
			networkings = append(networkings, out)
		}

		spec, err := svc.Spec()
		if err != nil {
			return err
		}

		scale := structs.ServiceScaleRequest{
			Arch:       spec.Arch,
			Users:      req.Users,
			Candidates: req.Candidates,
		}
		scale.Arch.Replicas += len(req.Units)

		if actor == nil {
			actor = alloc.NewAllocator(gd.Ormer(), gd.Cluster)
		}

		adds, err := gd.scaleUp(ctx, svc, actor, scale, networkings, false)
		if err != nil {
			return err
		}
		{
			err = svc.removeUnits(ctx, rm, nil)
			if err != nil {
				return err
			}

			err = migrateUnits(adds, rm, networkings)
			if err != nil {
				return err
			}
		}

		cms, err := svc.generateUnitsConfigs(ctx, nil)
		if err != nil {
			return err
		}

		err = svc.Compose(ctx, gd.PluginClient())
		if err != nil {
			return err
		}

		{

			list := make([]*unit, 0, len(rm))
			units, err := svc.getUnits()
			if err != nil {
				return err
			}

			for i := range rm {
				if u := getUnit(units, rm[i].u.ID); u != nil {
					list = append(list, u)
				}
			}

			err = registerUnits(ctx, list, gd.KVClient(), cms)
		}

		return err
	}

	task := database.NewTask(svc.svc.Name, database.UnitRebuildTask, svc.svc.ID, "", nil, 300)

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &task,
		statusServiceUnitRebuilding, statusServiceUnitRebuilt, statusServiceUnitRebuildFailed)

	err := sl.Run(isnotInProgress, rebuild, async)

	return task.ID, err
}

func migrateUnits(adds, rm []*unit, networkings [][]database.IP) error {
	type migrate struct {
		src, dest *unit
	}

	list := make([]migrate, 0, len(adds))

high:
	for i := range adds {
		ips, err := adds[i].uo.ListIPByUnitID(adds[i].u.ID)
		if err != nil {
			logrus.Errorf("%+v", err)
		}

		for j := range networkings {
			for k := range networkings[j] {

				for x := range ips {
					if ips[x].IPAddr == networkings[j][k].IPAddr {
						u := getUnit(rm, networkings[j][k].UnitID)

						list = append(list, migrate{
							src:  adds[i],
							dest: u,
						})

						continue high
					}
				}
			}
		}
	}

	// rename container name
	for i := range list {
		src, id, name := list[i].src, list[i].dest.u.ID, list[i].dest.u.Name

		err := renameContainer(src, list[i].dest.u.Name)
		if err != nil {
			return err
		}

		err = src.uo.MigrateUnit(src.u.ID, id, name)
		if err != nil {
			return err
		}
	}

	return nil
}

func renameContainer(u *unit, name string) error {
	e := u.getEngine()
	if e == nil {
		return errors.WithStack(newNotFound("Engine", u.u.EngineID))
	}

	c := u.getContainer()
	if c == nil {
		return errors.WithStack(newContainerError(u.u.Name, notFound))
	}

	err := e.RenameContainer(c, name)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
