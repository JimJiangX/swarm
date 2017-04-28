package garden

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"golang.org/x/net/context"
)

func (gd *Garden) Scale(ctx context.Context, svc *Service, actor alloc.Allocator, req structs.ServiceScaleRequest, async bool) (string, error) {
	scale := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		if req.Arch.Replicas == 0 || req.Arch.Replicas == len(units) {
			return nil
		}

		if len(units) > req.Arch.Replicas {
			err = svc.scaleDown(ctx, units, req.Arch.Replicas, gd.KVClient())
		} else {
			err = gd.scaleUp(ctx, svc, actor, req)
		}
		if err != nil {
			return err
		}

		{
			// update Service.Desc
			table, err := svc.so.GetService(svc.svc.ID)
			if err != nil {
				return err
			}
			desc := *table.Desc
			desc.ID = utils.Generate32UUID()
			desc.Replicas = req.Arch.Replicas
			desc.Previous = table.DescID

			out, err := json.Marshal(req.Arch)
			if err == nil {
				desc.Architecture = string(out)
			}

			table.DescID = desc.ID
			table.Desc = &desc

			err = svc.so.SetServiceDesc(table)

			return err
		}
	}

	task := database.NewTask(svc.svc.Name, database.ServiceScaleTask, svc.svc.ID, fmt.Sprintf("replicas=%d", req.Arch.Replicas), nil, 300)

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &task,
		statusServiceScaling, statusServiceScaled, statusServiceScaleFailed)

	err := sl.Run(isnotInProgress, scale, async)

	return task.ID, err
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

func (gd *Garden) scaleUp(ctx context.Context, svc *Service, actor alloc.Allocator, scale structs.ServiceScaleRequest) error {
	spec, err := svc.RefreshSpec()
	if err != nil {
		return err
	}
	if spec.Users == nil {
		spec.Users = scale.Users
	} else {
		spec.Users = append(spec.Users, scale.Users...)
	}

	if spec.Options == nil {
		spec.Options = make(map[string]interface{})
	}
	for k, v := range scale.Options {
		spec.Options[k] = v
	}

	// decode scheduleOption
	var opts scheduleOption
	r := strings.NewReader(svc.svc.Desc.ScheduleOptions)
	err = json.NewDecoder(r).Decode(&opts)
	if err != nil {
		return err
	}

	// adjust scheduleOption by unit
	units, err := svc.getUnits()
	if err != nil {
		return err
	}
	if len(units) > 0 {
		svc.options = adjustScheduleOptionsByUnits(opts, units)
	}

	auth, err := gd.AuthConfig()
	if err != nil {
		return err
	}

	// TODO:new units
	// TODO:inset add into DB
	add := database.Unit{}
	pu, err := gd.allocation(ctx, actor, svc, []database.Unit{add})
	if err != nil {
		return err
	}

	err = svc.runContainer(ctx, pu, auth)
	if err != nil {
		return err
	}

	err = svc.initStart(ctx, []*unit{newUnit(add, svc.so, svc.cluster)}, gd.KVClient(), nil, nil)

	return err
}

func adjustScheduleOptionsByUnits(opts scheduleOption, units []*unit) scheduleOption {
	// TODO: clusters\filters\networkings

	return opts
}
