package garden

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// UpdateImage update Service image version,
// stop units services,remove old containers,
// created & start new container with new version image.
func (svc *Service) UpdateImage(ctx context.Context, kvc kvstore.Client,
	im database.Image, task *database.Task, async bool, authConfig *types.AuthConfig) error {

	update := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		version, err := getImage(svc.so, im.Image())
		if err != nil {
			return err
		}

		changes := make([]*unit, 0, len(units))

		for _, u := range units {

			c := u.getContainer()
			if c == nil || c.Engine == nil {
				return errors.WithStack(newContainerError(u.u.Name, notFound))
			}

			if c.Config.Image == version {
				continue
			}

			changes = append(changes, u)
		}

		if len(changes) == 0 {
			return nil
		}

		err = svc.stop(ctx, units, true)
		if err != nil {
			return err
		}

		containers := make([]struct {
			u  database.Unit
			nc *cluster.Container
		}, 0, len(changes))

		for _, u := range changes {
			c := u.getContainer()
			if c == nil || c.Engine == nil {
				return errors.WithStack(newContainerError(u.u.Name, notFound))
			}

			if c.Config.Image == version {
				continue
			}

			c.Config.Image = version
			// set new swarmID
			swarmID := utils.Generate32UUID()
			c.Config.SetSwarmID(swarmID)
			c.Config.Config.Labels["mgm.unit.type"] = im.Name
			c.Config.Config.Labels["mgm.image.id"] = im.ID

			nc, err := c.Engine.CreateContainer(c.Config, swarmID, true, authConfig)
			if err != nil {
				return errors.WithStack(err)
			}

			containers = append(containers, struct {
				u  database.Unit
				nc *cluster.Container
			}{u.u, nc})
		}

		for i, c := range containers {
			origin := svc.cluster.Container(c.u.ContainerID)
			if origin != nil {
				err = origin.Engine.RemoveContainer(origin, true, false)
			}

			if err == nil {
				err = c.nc.Engine.RenameContainer(c.nc, c.u.Name)
			}
			if err != nil {
				return errors.WithStack(err)
			}

			c := svc.cluster.Container(c.u.Name)
			containers[i].nc = c
			containers[i].u.ContainerID = c.ID
		}

		{
			// ensure save new ContainerID
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
			// start units services
			_, err := svc.RefreshSpec()
			if err != nil {
				return err
			}

			err = svc.start(ctx, nil, nil)
			if err != nil {
				return err
			}
		}

		{
			// save new Container to KV
			for i := range containers {
				err := saveContainerToKV(ctx, kvc, containers[i].nc)
				if err != nil {
					return err
				}
			}
		}

		{
			table, err := svc.so.GetService(svc.ID())
			if err != nil {
				return err
			}

			table = updateDescByImage(table, im)

			err = svc.so.SetServiceDesc(table)
			if err == nil {
				svc.svc = &table
			}

			return err
		}
	}

	tl := tasklock.NewServiceTask(
		database.ServiceUpdateImageTask,
		svc.ID(), svc.so, task,
		statusServiceImageUpdating,
		statusServiceImageUpdated,
		statusServiceImageUpdateFailed)

	return tl.Run(isnotInProgress, update, async)
}

func updateDescByImage(table database.Service, im database.Image) database.Service {
	desc := *table.Desc
	desc.ID = utils.Generate32UUID()
	desc.Image = im.Image()
	desc.ImageID = im.ImageID
	desc.Previous = table.DescID

	table.DescID = desc.ID
	table.Desc = &desc

	return table

}

// UpdateResource udpate service containers CPU & memory settings.
func (svc *Service) UpdateResource(ctx context.Context, actor alloc.Allocator, ncpu, memory *int64) error {
	desc := svc.svc.Desc

	if ncpu == nil {
		n := int64(desc.NCPU)
		ncpu = &n
	}

	if memory == nil {
		memory = &desc.Memory
	}

	update := func() error {
		type pending struct {
			u      *unit
			cpuset string
			memory int64

			config container.UpdateConfig
		}

		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		pendings := make([]pending, 0, len(units))

		for _, u := range units {
			var (
				nccpu    int64 // container HostConfig.CpusetCpus
				countCPU bool  // set true after called CountCPU
			)

			c := u.getContainer()
			if c == nil || c.Engine == nil {
				return errors.WithStack(newContainerError(u.u.Name, notFound))
			}

			if c.Config.HostConfig.Memory == 0 && *memory != 0 {
				return errors.Errorf("Forbid updating container memory from unlimited 0 to %d", *memory)
			}

			if c.Config.HostConfig.Memory == *memory {
				nccpu, err = c.Config.CountCPU()
				if err != nil {
					return errors.WithStack(err)
				}

				if nccpu == *ncpu {
					continue
				}

				countCPU = true
			}

			pu := pending{
				u:      u,
				memory: *memory,
				cpuset: c.Config.HostConfig.CpusetCpus,
			}

			if !countCPU {
				nccpu, err = c.Config.CountCPU()
				if err != nil {
					return errors.WithStack(err)
				}
			}

			if nccpu > *ncpu {
				pu.cpuset, err = reduceCPUset(c.Config.HostConfig.CpusetCpus, int(*ncpu))
				if err != nil {
					return err
				}
			}

			if c.Config.HostConfig.Memory < *memory || nccpu < *ncpu {
				node := node.NewNode(c.Engine)

				cpuset, err := actor.AlloctCPUMemory(c.Config, node, *ncpu-nccpu, *memory-c.Config.HostConfig.Memory, nil)
				if err != nil {
					return err
				}
				if cpuset != "" {
					if pu.cpuset == "" {
						pu.cpuset = cpuset
					} else {
						pu.cpuset = pu.cpuset + "," + cpuset
					}
				}
			}

			pu.config = container.UpdateConfig{
				Resources: container.Resources{
					CpusetCpus: pu.cpuset,
					Memory:     pu.memory,
					MemorySwap: int64(float64(pu.memory) * 1.5),
				},
			}

			pendings = append(pendings, pu)
		}

		if len(pendings) == 0 {
			// no cpu&memory update
			return nil
		}

		for _, pu := range pendings {
			err = pu.u.update(ctx, pu.config)
			if err != nil {
				return err
			}
		}

		err = updateConfigAfterUpdateResource(ctx, svc, units, memory)
		if err != nil {
			return err
		}

		{
			// update Service.Desc
			table, err := svc.so.GetService(svc.ID())
			if err != nil {
				return err
			}

			table, err = updateDescByResource(table, *ncpu, *memory)
			if err != nil {
				return err
			}

			err = svc.so.SetServiceDesc(table)
			if err == nil {
				svc.svc = &table
			}

			return err
		}
	}

	sl := tasklock.NewServiceTask(database.ServiceUpdateTask+"_cpu", svc.ID(), svc.so, nil,
		statusServiceResourceUpdating, statusServiceResourceUpdated, statusServiceResourceUpdateFailed)

	return sl.Run(isnotInProgress, update, false)
}

func updateConfigAfterUpdateResource(ctx context.Context, svc *Service, units []*unit, memory *int64) error {
	cms, err := svc.ReloadServiceConfig(ctx, "")
	if err != nil {
		return err
	}

	if memory == nil || (svc.spec.Image.Name != "upsql" && svc.spec.Image.Name != "upredis") {
		return nil
	}

	logrus.Debugf("updateConfigAfterUpdateResource:'%s'", svc.spec.Image.Name)

	// update units config file but whether start by user
	err = svc.updateConfigs(ctx, units, cms, nil)
	if err != nil {
		return err
	}

	kv := kvPair{}

	if strings.Contains(svc.spec.Image.Name, "upreids") {

		kv.key = "maxmemory"
		kv.value = strconv.Itoa(int(float64(*memory) * 0.75))

	} else {
		n := *memory
		if n>>33 > 0 { // 8G
			n = int64(float64(n) * 0.70)
		} else {
			n = int64(float64(n) * 0.5)
		}

		kv.key = "mysqld::innodb_buffer_pool_size"
		kv.value = strconv.Itoa(int(n))
	}

	return effectServiceConfig(ctx, units, svc.spec.Image.Name, []kvPair{kv})
}

type kvPair struct {
	key   string
	value string
}

func effectServiceConfig(ctx context.Context, units []*unit, imageName string, pairs []kvPair) error {
	cmd := make([]string, 2, 2+len(pairs))
	cmd[0] = "/root/effect-config.sh"
	cmd[1] = imageName

	for i := range pairs {
		cmd = append(cmd, fmt.Sprintf("%s=%s", pairs[i].key, pairs[i].value))
	}

	for i := range units {
		_, err := units[i].containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateDescByResource(table database.Service, ncpu, memory int64) (database.Service, error) {
	desc := *table.Desc
	desc.ID = utils.Generate32UUID()
	desc.NCPU = int(ncpu)
	desc.Memory = memory

	schedOpts := scheduleOption{}
	r := strings.NewReader(table.Desc.ScheduleOptions)
	err := json.NewDecoder(r).Decode(&schedOpts)
	if err != nil && table.Desc.ScheduleOptions != "" {
		return table, errors.WithStack(err)
	}

	schedOpts.Require.Require.CPU = desc.NCPU
	schedOpts.Require.Require.Memory = desc.Memory

	sr, err := json.Marshal(schedOpts)
	if err != nil {
		return table, errors.WithStack(err)
	}

	desc.ScheduleOptions = string(sr)
	desc.Previous = table.DescID

	table.DescID = desc.ID
	table.Desc = &desc

	return table, nil
}

func reduceCPUset(cpusetCpus string, need int) (string, error) {
	cpus, err := utils.ParseUintList(cpusetCpus)
	if err != nil {
		return "", errors.Wrap(err, "parse cpusetCpus:"+cpusetCpus)
	}

	cpuSlice := make([]int, 0, len(cpus))
	for k, ok := range cpus {
		if ok {
			cpuSlice = append(cpuSlice, k)
		}
	}

	if len(cpuSlice) < need {
		return cpusetCpus, errors.Errorf("%s is shortage for need %d", cpusetCpus, need)
	}

	sort.Ints(cpuSlice)

	cpuString := make([]string, need)
	for n := 0; n < need; n++ {
		cpuString[n] = strconv.Itoa(cpuSlice[n])
	}

	return strings.Join(cpuString, ","), nil
}

// VolumeExpansion expand container volume size.
func (svc *Service) VolumeExpansion(actor alloc.Allocator, target []structs.VolumeRequire) error {
	if len(target) == 0 {
		return nil
	}

	expansion := func() error {
		opts, err := svc.getScheduleOption()
		if err != nil {
			return err
		}

		target, err = mergeVolumeRequire(opts.Require.Volumes, target)
		if err != nil {
			return err
		}

		type pending struct {
			u        *unit
			eng      *cluster.Engine
			add      []structs.VolumeRequire
			creating []database.Volume
		}

		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		pendings := make([]pending, 0, len(units))

		// check node which unit on whether disk has enough free size.
		for _, u := range units {
			eng := u.getEngine()
			if eng == nil {
				return errors.WithStack(newNotFound("Engine", u.u.EngineID))
			}

			add, creating, err := u.prepareExpandVolume(eng, target)
			if err != nil {
				return err
			}

			err = actor.IsNodeStoreEnough(eng, add)
			if err != nil {
				return err
			}

			pendings = append(pendings, pending{
				u:        u,
				eng:      eng,
				add:      add,
				creating: creating,
			})
		}

		// expand volume size
		for _, pu := range pendings {
			for i := range pu.creating {
				err := engineCreateVolume(pu.eng, pu.creating[i])
				if err != nil {
					return err
				}
			}

			err := actor.ExpandVolumes(pu.eng, pu.add)
			if err != nil {
				return err
			}
		}

		{
			// update Service.Desc
			table, err := svc.so.GetService(svc.ID())
			if err != nil {
				return err
			}

			table, err = updateDescByVolumeReuires(table, &opts, target)
			if err != nil {
				return err
			}

			err = svc.so.SetServiceDesc(table)
			if err == nil {
				svc.svc = &table
			}

			return err
		}
	}

	sl := tasklock.NewServiceTask(database.ServiceUpdateTask+"_lv", svc.ID(), svc.so, nil,
		statusServiceVolumeExpanding, statusServiceVolumeExpanded, statusServiceVolumeExpandFailed)

	return sl.Run(isnotInProgress, expansion, false)
}

func updateDescByVolumeReuires(table database.Service, opts *scheduleOption, target []structs.VolumeRequire) (database.Service, error) {
	if opts == nil {
		opts = &scheduleOption{}
	}
	opts.Require.Volumes = target

	sr, err := json.Marshal(opts)
	if err != nil {
		return table, errors.WithStack(err)
	}

	vb, err := json.Marshal(target)
	if err != nil {
		return table, errors.WithStack(err)
	}

	desc := *table.Desc
	desc.ID = utils.Generate32UUID()
	desc.Volumes = string(vb)
	desc.ScheduleOptions = string(sr)
	desc.Previous = table.DescID

	table.DescID = desc.ID
	table.Desc = &desc

	return table, nil
}

func mergeVolumeRequire(old, update []structs.VolumeRequire) ([]structs.VolumeRequire, error) {
	if len(old) == 0 {

		for v := range update {
			if update[v].Name == "" || update[v].Type == "" {
				return nil, errors.Errorf("invalid volume require,%v", update[v])
			}
		}

		return update, nil
	}

	out := make([]structs.VolumeRequire, len(old))
	copy(out, old)

loop:
	for v := range update {

		for i := range out {
			if out[i].Name == update[v].Name {

				out[i].Size = update[v].Size

				continue loop
			}
		}

		if update[v].Name == "" || update[v].Type == "" {
			return nil, errors.Errorf("invalid volume require,%v", update[v])
		}

		out = append(out, update[v])
	}

	return out, nil
}

func (svc *Service) UpdateNetworking(ctx context.Context, actor alloc.Allocator, require []structs.NetDeviceRequire) error {
	if len(require) == 0 {
		return nil
	}

	width := 0
	for i := range require {
		if require[i].Bandwidth > width {
			width = require[i].Bandwidth
		}
	}

	update := func() error {
		units, err := svc.getUnits()
		if err != nil {
			return err
		}

		for _, u := range units {
			eng := u.getEngine()
			if eng == nil {
				return errors.WithStack(newNotFound("Engine", u.u.EngineID))
			}

			ips, err := u.uo.ListIPByUnitID(u.u.ID)
			if err != nil {
				return err
			}

			err = actor.UpdateNetworking(ctx, eng.ID, ips, width)
			if err != nil {
				return err
			}
		}

		{
			// update Service.Desc
			table, err := svc.so.GetService(svc.ID())
			if err != nil {
				return err
			}

			table, err = updateDescByNetworkReuires(table, nil, require)
			if err != nil {
				return err
			}

			err = svc.so.SetServiceDesc(table)
			if err == nil {
				svc.svc = &table
			}

			return err
		}
	}

	sl := tasklock.NewServiceTask(database.ServiceUpdateTask+"_net", svc.ID(), svc.so, nil,
		statusServiceNetworkUpdating, statusServiceNetworkUpdated, statusServiceNetworkUpdateFailed)

	return sl.Run(isnotInProgress, update, false)
}

func updateDescByArch(table database.Service, arch structs.Arch) database.Service {
	desc := *table.Desc
	desc.ID = utils.Generate32UUID()
	desc.Replicas = arch.Replicas
	desc.Previous = table.DescID

	out, err := json.Marshal(arch)
	if err == nil {
		desc.Architecture = string(out)
	}

	table.DescID = desc.ID
	table.Desc = &desc

	return table
}

func updateDescByNetworkReuires(table database.Service, opts *scheduleOption, target []structs.NetDeviceRequire) (database.Service, error) {
	if opts == nil {
		opts = &scheduleOption{}
		r := strings.NewReader(table.Desc.ScheduleOptions)
		err := json.NewDecoder(r).Decode(opts)
		if err != nil && table.Desc.ScheduleOptions != "" {
			return table, errors.WithStack(err)
		}
	}

	opts.Require.Networks = target

	sr, err := json.Marshal(opts)
	if err != nil {
		return table, errors.WithStack(err)
	}

	nb, err := json.Marshal(target)
	if err != nil {
		return table, errors.WithStack(err)
	}

	desc := *table.Desc
	desc.ID = utils.Generate32UUID()
	desc.Networks = string(nb)
	desc.ScheduleOptions = string(sr)
	desc.Previous = table.DescID

	table.DescID = desc.ID
	table.Desc = &desc

	return table, nil
}
