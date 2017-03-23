package garden

import (
	"sort"

	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (svc *Service) ScaleDown(ctx context.Context, reg kvstore.Register, replicas int) (err error) {
	scale := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		if len(units) > replicas {
			err = svc.scaleDown(ctx, units, replicas, reg)
		}

		return err
	}

	sl := tasklock.NewServiceTask(svc.spec.ID, svc.so, nil,
		statusServiceScaling, statusServiceScaled, statusServiceScaleFailed)

	return sl.Run(isnotInProgress, scale)
}

func (svc *Service) scaleDown(ctx context.Context, units []*unit, replicas int, reg kvstore.Register) error {
	containers := svc.cluster.Containers()
	out := sortUnitsByContainers(units, containers)

	stoped := out[:len(units)-replicas]

	err := svc.deregisterSerivces(ctx, reg, stoped)
	if err != nil {
		return err
	}

	err = svc.removeContainers(ctx, stoped, true, false)
	if err != nil {
		return err
	}

	err = svc.removeVolumes(ctx, stoped)
	if err != nil {
		return err
	}

	list := make([]database.Unit, 0, len(stoped))
	for i := range stoped {
		if stoped[i] == nil {
			continue
		}

		list = append(list, stoped[i].u)
	}

	err = svc.so.DelUnitsRelated(list, true)

	return err
}

type unitStatus struct {
	u     *unit
	c     *cluster.Container
	score int
}

type byStatus []unitStatus

func (b byStatus) Len() int {
	return len(b)
}

func (b byStatus) Less(i, j int) bool {
	return b[i].score < b[j].score
}

func (b byStatus) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func sortUnitsByContainers(units []*unit, containers cluster.Containers) []*unit {
	var list byStatus = make([]unitStatus, len(units))

	for i := range units {
		score := 0
		c := containers.Get(units[i].u.Name)

		// StateString from container.go L23
		switch {
		case c == nil:
			score = 0
		case c.State == "removing":
			score = 1
		case c.State == "dead":
			score = 2
		case c.State == "exited":
			score = 3
		case c.State == "created":
			score = 4
		case c.State != "running":
			score = 5
		case c.State == "running":
			score = 10
		default:
			score = 4
		}

		list[i] = unitStatus{
			u:     units[i],
			c:     c,
			score: score,
		}
	}

	sort.Sort(list)

	out := make([]*unit, len(units))
	for i := range list {
		out[i] = list[i].u
	}

	return out
}

func (svc *Service) UpdateImage(ctx context.Context, kvc kvstore.Client,
	im database.Image, task database.Task,
	authConfig *types.AuthConfig) error {

	update := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		_, version, err := getImage(svc.so, im.ID)
		if err != nil {
			return err
		}

		err = svc.stop(ctx, units, true)
		if err != nil {
			return err
		}

		containers := make([]struct {
			u  database.Unit
			nc *cluster.Container
		}, 0, len(units))

		for _, u := range units {
			c := u.getContainer()

			if c == nil || c.Engine == nil {
				return errors.Wrap(newContainerError(u.u.Name, "not found"), "rebuild container")
			}
			c.Config.Image = version

			nc, err := c.Engine.CreateContainer(c.Config, u.u.Name+"-"+version, true, authConfig)
			if err != nil {
				return err
			}

			containers = append(containers, struct {
				u  database.Unit
				nc *cluster.Container
			}{u.u, nc})
		}

		for i, c := range containers {
			origin := svc.cluster.Container(c.u.ContainerID)
			if origin == nil {
				continue
			}

			err := origin.Engine.RemoveContainer(origin, true, false)
			if err == nil {

				if err = c.nc.Engine.RenameContainer(c.nc, c.u.Name); err == nil {
					err = c.nc.Engine.StartContainer(c.nc, nil)
				}
			}
			if err != nil {
				return err
			}

			c := svc.cluster.Container(c.u.Name)
			containers[i].nc = c
			containers[i].u.ContainerID = c.ID
		}

		{
			// save new ContainerID
			in := make([]database.Unit, 0, len(containers))
			for i := range containers {
				in = append(in, containers[i].u)
			}

			err = svc.so.SetUnits(in)
			if err != nil {
				return err
			}
		}

		{
			// save new Container to KV
			for i := range containers {
				err := saveContainerToKV(kvc, containers[i].nc)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}

	tl := tasklock.NewServiceTask(svc.spec.ID, svc.so, &task,
		statusServiceImageUpdating,
		statusServiceImageUpdated,
		statusServiceImageUpdateFailed)

	return tl.Run(isnotInProgress, update)
}
