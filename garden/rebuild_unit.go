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

	task := database.NewTask(svc.Name(), database.UnitRebuildTask, svc.ID(), "", nil, 300)

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, &task,
		statusServiceUnitRebuilding, statusServiceUnitRebuilt, statusServiceUnitRebuildFailed)

	err := sl.Run(isnotInProgress, func() error {
		return gd.rebuildUnit(ctx, svc, req.NameOrID, req.Candidates, false)
	}, async)

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
