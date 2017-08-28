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

		err = svc.Compose(ctx, gd.PluginClient())

		err = svc.removeUnits(ctx, rm, nil)
		if err != nil {
			return err
		}

		err = migrateUnits(svc.so, adds, networkings)

		return err
	}

	task := database.NewTask(svc.svc.Name, database.UnitRebuildTask, svc.svc.ID, "", nil, 300)

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &task,
		statusServiceUnitRebuilding, statusServiceUnitRebuilt, statusServiceUnitRebuildFailed)

	err := sl.Run(isnotInProgress, rebuild, async)

	return task.ID, err
}

func migrateUnits(orm database.UnitOrmer, adds []*unit, networkings [][]database.IP) error {
	type migrateID struct {
		src, dest string
	}

	list := make([]migrateID, 0, len(adds))

high:
	for i := range adds {
		ips, err := orm.ListIPByUnitID(adds[i].u.ID)
		if err != nil {
			logrus.Errorf("%+v", err)
		}

		for j := range networkings {
			for k := range networkings[j] {

				for x := range ips {
					if ips[x].IPAddr == networkings[j][k].IPAddr {
						list = append(list, migrateID{
							src:  ips[x].UnitID,
							dest: networkings[j][k].UnitID,
						})

						continue high
					}
				}
			}
		}
	}

	for i := range list {
		err := orm.MigrateUnit(list[i].src, list[i].dest)
		if err != nil {
			return err
		}
	}

	return nil
}
