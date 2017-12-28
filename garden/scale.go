package garden

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Scale is exported.
// 服务水平扩展，增加或者减少节点。
// 减少节点，移除的顺序排列，按容器顺序：
// 		not exist -- removing -- dead -- exited -- created -- running
//
// 增加节点：
//      调试准备 -- 调度 -- 分配资源 -- 创建容器 -- 启动容器与服务
func (gd *Garden) Scale(ctx context.Context, svc *Service, actor alloc.Allocator, req structs.ServiceScaleRequest, async bool) (structs.ServiceScaleResponse, error) {
	var add []database.Unit
	resp := structs.ServiceScaleResponse{}

	units, err := svc.getUnits()
	if err != nil {
		return resp, err
	}

	if n := req.Arch.Replicas - len(units); n > 0 {
		add, err = svc.addNewUnit(n)
		if err != nil {
			return resp, err
		}

		resp.Add = make([]structs.UnitNameID, 0, len(add))

		for i := range add {
			resp.Add = append(resp.Add, structs.UnitNameID{
				ID:   add[i].ID,
				Name: add[i].Name,
			})
		}
	}

	scale := func() error {
		if req.Arch.Replicas == 0 || req.Arch.Replicas == len(units) {
			return nil
		}

		if len(units) > req.Arch.Replicas {
			err = svc.scaleDown(ctx, units, req.Arch.Replicas, gd.KVClient())
		} else {
			_, err = gd.scaleUp(ctx, svc, actor, serviceScaleRequest{
				ServiceScaleRequest: req,
				Units:               add,
			}, nil, true, true, true)
		}
		if err != nil {
			return err
		}

		{
			// update Service.Desc
			table, err := svc.so.GetService(svc.ID())
			if err != nil {
				return err
			}

			table = updateDescByArch(table, req.Arch)

			err = svc.so.SetServiceDesc(table)
			if err != nil {
				return err
			}
		}

		if req.Compose {
			err = svc.Compose(ctx, gd.PluginClient())
		}

		return err
	}

	task := database.NewTask(svc.Name(), database.ServiceScaleTask, svc.ID(), fmt.Sprintf("replicas=%d", req.Arch.Replicas), nil, 300)

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, &task,
		statusServiceScaling, statusServiceScaled, statusServiceScaleFailed)

	err = sl.Run(isnotInProgress, scale, async)

	resp.Task = task.ID

	return resp, err
}

func (svc *Service) scaleDown(ctx context.Context, units []*unit, replicas int, reg kvstore.Register) error {
	containers := svc.cluster.Containers()
	out := sortUnitsByContainers(units, containers)

	rm := out[:len(units)-replicas]

	return svc.removeUnits(ctx, rm, reg)
}

type serviceScaleRequest struct {
	structs.ServiceScaleRequest
	Units []database.Unit
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
			score = 0 // not exist,maybe the engine is down
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

func (gd *Garden) scaleAllocation(ctx context.Context, svc *Service, actor alloc.Allocator,
	vr, nr bool, add []database.Unit, candidates []string,
	options map[string]interface{}) ([]*unit, []pendingUnit, error) {

	err := svc.prepareSchedule(candidates, options)
	if err != nil {
		return nil, nil, err
	}

	adds := make([]*unit, len(add))
	for i := range add {
		adds[i] = newUnit(add[i], svc.so, svc.cluster)
	}

	pendings, err := gd.allocation(ctx, actor, svc, add, vr, nr)
	if err != nil {
		return adds, nil, err
	}

	for i := range add {
		adds[i] = newUnit(pendings[i].Unit, svc.so, svc.cluster)
	}

	return adds, pendings, err
}

func (gd *Garden) scaleUp(ctx context.Context, svc *Service, actor alloc.Allocator,
	scale serviceScaleRequest, networkings [][]database.IP, vr, nr, register bool) ([]*unit, error) {

	units, pendings, err := gd.scaleAllocation(ctx, svc, actor, vr, nr,
		scale.Units, scale.Candidates, scale.Options)
	defer func() {
		if err != nil {
			_err := svc.removeUnits(ctx, units, gd.kvClient)
			if _err != nil {
				err = errors.Errorf("%+v\nremove new addition units:%+v", err, _err)
			}
		}
	}()
	if err != nil {
		return units, err
	}
	if !nr && len(networkings) > 0 {
		if len(pendings) > len(networkings) {
			return units, errors.Errorf("not enough networkings for addition units")
		}

		defer func() {
			if err != nil {
				list := make([]database.IP, 0, len(networkings)*2)
				for i := range networkings {
					list = append(list, networkings[i]...)
				}
				_err := svc.so.SetIPs(list)
				if _err != nil {
					err = errors.Errorf("%+v recover networking settings error:%+v", err, _err)
				}
			}
		}()

		for i, pu := range pendings {
			pendings[i].networkings, err = actor.AllocDevice(pu.Unit.EngineID, pu.Unit.ID, networkings[i])
			if err != nil {
				return units, err
			}
		}
	}

	auth, err := gd.AuthConfig()
	if err != nil {
		return units, err
	}

	err = svc.runContainer(ctx, pendings, false, auth)
	if err != nil {
		return units, err
	}

	kvc := gd.KVClient()
	if !register {
		kvc = nil
	}

	err = svc.initStart(ctx, units, kvc, nil, nil)

	return units, err
}

func (svc *Service) prepareSchedule(candidates []string, options map[string]interface{}) error {
	spec, err := svc.RefreshSpec()
	if err != nil {
		return err
	}

	opts, err := svc.getScheduleOption()
	if err != nil {
		return err
	}

	if len(candidates) > 0 {
		constraints := fmt.Sprintf("%s==%s", engineLabel, strings.Join(candidates, "|"))
		svc.options.Nodes.Constraints = append(svc.options.Nodes.Constraints, constraints)
	}

	units, err := svc.getUnits()
	if err != nil {
		return err
	}

	// adjust scheduleOption by unit
	svc.options, err = scheduleOptionsByUnits(opts, units, len(candidates) <= 0)
	if err != nil {
		return err
	}

	if spec.Options == nil && options != nil {
		spec.Options = options

		return nil
	}

	for k, v := range options {
		spec.Options[k] = v
	}

	return nil
}

func (svc *Service) addNewUnit(num int) ([]database.Unit, error) {
	spec := svc.spec

	now := time.Now()
	add := make([]database.Unit, num)

	for i := range add {
		uid := utils.Generate32UUID()
		add[i] = database.Unit{
			ID:          uid,
			Name:        fmt.Sprintf("%s_%s", uid[:8], spec.Tag), // <unit_id_8bit>_<service_tag>
			Type:        spec.Image.Name,
			ServiceID:   spec.ID,
			NetworkMode: "none",
			Status:      0,
			CreatedAt:   now,
		}
	}

	err := svc.so.InsertUnits(add)

	return add, err
}

func (svc *Service) getScheduleOption() (scheduleOption, error) {
	// decode scheduleOption
	opts := scheduleOption{}

	r := strings.NewReader(svc.svc.Desc.ScheduleOptions)
	err := json.NewDecoder(r).Decode(&opts)
	if err == nil {
		svc.options = opts
	}

	return svc.options, err
}

func scheduleOptionsByUnits(opts scheduleOption, units []*unit, filter bool) (scheduleOption, error) {
	if len(units) == 0 {
		return opts, nil
	}

	unit := units[0]

	node, err := unit.uo.GetNode(unit.u.EngineID)
	if err != nil {
		return opts, err
	}

	opts.Nodes.Clusters = []string{node.ClusterID}

	ips, err := unit.uo.ListIPByUnitID(unit.u.ID)
	if err != nil {
		return opts, err
	}

	if len(ips) == 1 {
		opts.Nodes.Networkings = map[string][]string{node.ClusterID: {ips[0].Networking}}
	} else {
		ids := make([]string, 0, len(ips))

	loop:
		for i := range ips {

			for j := range ids {
				if ids[j] == ips[i].Networking {
					continue loop
				}
			}

			ids = append(ids, ips[i].Networking)
		}

		opts.Nodes.Networkings = map[string][]string{node.ClusterID: ids}
	}

	if filter {
		filters := make([]string, 0, len(units))
		for i := range units {
			filters = append(filters, units[i].u.EngineID)
		}

		opts.Nodes.Filters = append(opts.Nodes.Filters, filters...)
	}

	return opts, err
}
