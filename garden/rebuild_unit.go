package garden

import (
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (gd *Garden) RebuildUnits(ctx context.Context, svc *Service, assign []string, actor alloc.Allocator, users []structs.User, async bool) (string, error) {
	rebuild := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		rm := make([]*unit, 0, len(assign))
		for _, id := range assign {
			found := false
			for _, u := range units {
				if u.u.ID == id || u.u.Name == id || u.u.ContainerID == id {
					rm = append(rm, u)
					found = true
					break
				}
			}
			if !found {
				return errors.Errorf("unit '%s' isnot belong to Service '%s'", id, svc.svc.Name)
			}
		}

		spec, err := svc.Spec()
		if err != nil {
			return err
		}

		req := structs.ServiceScaleRequest{
			Arch:  spec.Arch,
			Users: users,
		}
		req.Arch.Replicas += len(assign)

		if actor == nil {
			actor = alloc.NewAllocator(gd.Ormer(), gd.Cluster)
		}

		err = gd.scaleUp(ctx, svc, actor, req)
		if err != nil {
			return err
		}

		err = svc.removeUnits(ctx, rm, gd.KVClient())

		return err
	}

	task := database.NewTask(svc.svc.Name, database.UnitRebuildTask, svc.svc.ID, "", nil, 300)

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &task,
		statusServiceUnitRebuilding, statusServiceUnitRebuilt, statusServiceUnitRebuildFailed)

	err := sl.Run(isnotInProgress, rebuild, async)

	return task.ID, err
}
