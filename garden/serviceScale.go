package garden

import (
	"encoding/json"
	"sort"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (svc *Service) ScaleDown(ctx context.Context, table database.Service, reg kvstore.Register, replicas int) (err error) {
	ok, val, err := svc.sl.CAS(statusServiceScaling, isnotInProgress)
	if err != nil {
		return err
	}

	svc.spec.Status = val

	if !ok {
		return errors.Wrap(newStatusError(statusServiceScaling, val), "Service scale")
	}

	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("panic:%v", r)
		}

		if err != nil {
			table.Status = statusServiceScaleFailed
		}

		// TODO:database.NewTask
		_err := svc.so.SetServiceScale(table, database.Task{})
		if _err != nil {
			logrus.WithField("Service", svc.spec.Name).Errorf("orm:Set Service status:%d,%+v", table.Status, _err)
		}
	}()

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	if len(units) > replicas {
		err = svc.scaleDown(ctx, units, replicas, reg)
		if err != nil {
			return err
		}
	}

	arch := svc.spec.Arch
	arch.Replicas = replicas

	out, err := json.Marshal(arch)
	if err == nil {
		table.Desc.Replicas = replicas
		table.Desc.Architecture = string(out)
	}

	table.Status = statusServiceScaled

	return nil
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
