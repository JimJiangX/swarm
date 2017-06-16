package garden

import (
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

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

		scale := structs.ServiceScaleRequest{
			Arch:       spec.Arch,
			Users:      req.Users,
			Candidates: req.Candidates,
		}
		scale.Arch.Replicas += len(req.Units)

		if actor == nil {
			actor = alloc.NewAllocator(gd.Ormer(), gd.Cluster)
		}

		err = gd.scaleUp(ctx, svc, actor, scale)
		if err != nil {
			return err
		}

		err = svc.removeUnits(ctx, rm, gd.KVClient())
		if err != nil {
			return err
		}

		err = svc.Compose(ctx, gd.PluginClient())

		return err
	}

	task := database.NewTask(svc.svc.Name, database.UnitRebuildTask, svc.svc.ID, "", nil, 300)

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &task,
		statusServiceUnitRebuilding, statusServiceUnitRebuilt, statusServiceUnitRebuildFailed)

	err := sl.Run(isnotInProgress, rebuild, async)

	return task.ID, err
}
