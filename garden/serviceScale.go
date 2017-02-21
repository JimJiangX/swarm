package garden

import (
	"fmt"
	"sort"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (svc *Service) Scale(ctx context.Context, r kvstore.Register, replicas int) error {
	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	if len(units) == replicas {
		return nil
	}

	ok, val, err := svc.sl.CAS(statusServiceScaling, isInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceScaling, val), "Service scale")
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		status := statusServiceScaled
		if err != nil {
			status = statusServiceScaleFailed
		}

		err := svc.sl.SetStatus(status)
		if err != nil {

		}
	}()

	if len(units) > replicas {
		containers := svc.cluster.Containers()
		out := sortUnitsByContainers(units, containers)

		stoped := out[:len(units)-replicas]

		err = svc.deregisterSerivces(ctx, r, stoped)
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

		// del data by unit.u.ID
	}

	return nil
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
