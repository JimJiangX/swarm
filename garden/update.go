package garden

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

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
				return errors.WithStack(newContainerError(u.u.Name, "not found"))
			}
			c.Config.Image = version
			c.Config.SetSwarmID(utils.Generate32UUID())

			nc, err := c.Engine.CreateContainer(c.Config, u.u.Name+"-"+version, true, authConfig)
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
				if err = c.nc.Engine.RenameContainer(c.nc, c.u.Name); err == nil {
					err = c.nc.Engine.StartContainer(c.nc, nil)
				}
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

			err = svc.start(ctx, nil, nil, nil)
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

		{
			table, err := svc.so.GetService(svc.svc.ID)
			if err != nil {
				return err
			}
			desc := *table.Desc
			desc.ID = utils.Generate32UUID()
			desc.Image = im.Version()
			desc.ImageID = im.ImageID
			desc.Previous = table.DescID

			table.DescID = desc.ID
			table.Desc = &desc

			err = svc.so.SetServiceDesc(table)
			if err == nil {
				svc.svc = &table
			}

			return err
		}
	}

	tl := tasklock.NewServiceTask(svc.svc.ID, svc.so, task,
		statusServiceImageUpdating,
		statusServiceImageUpdated,
		statusServiceImageUpdateFailed)

	return tl.Run(isnotInProgress, update, async)
}

// UpdateResource udpate service containers CPU & memory settings.
func (svc *Service) UpdateResource(ctx context.Context, actor alloc.Allocator, ncpu, memory int64) error {
	desc := svc.svc.Desc

	if (ncpu == 0 || int64(desc.NCPU) == ncpu) &&
		(memory == 0 || desc.Memory == memory) {
		// nothing change on CPU & Memory
		return nil
	}

	if ncpu == 0 {
		ncpu = int64(desc.NCPU)
	}

	if memory == 0 {
		memory = desc.Memory
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

			c := u.getContainer()
			if c == nil {
				continue
			}

			pu := pending{
				u:      u,
				memory: memory,
				cpuset: c.Config.HostConfig.CpusetCpus,
			}

			n, err := c.Config.CountCPU()
			if err != nil {
				return errors.WithStack(err)
			}

			if n > ncpu {
				pu.cpuset, err = reduceCPUset(c.Config.HostConfig.CpusetCpus, int(ncpu))
				if err != nil {
					return err
				}
			}

			if c.Config.HostConfig.Memory < memory || n < ncpu {
				node := node.NewNode(c.Engine)

				cpuset, err := actor.AlloctCPUMemory(c.Config, node, ncpu-n, memory-c.Config.HostConfig.Memory, nil)
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
				},
			}

			pendings = append(pendings, pu)
		}

		for _, pu := range pendings {
			err = pu.u.update(ctx, pu.config)
			if err != nil {
				return err
			}
		}

		// units config file updated by user

		{
			// update Service.Desc
			table, err := svc.so.GetService(svc.svc.ID)
			if err != nil {
				return err
			}
			desc := *table.Desc
			desc.ID = utils.Generate32UUID()
			desc.NCPU = int(ncpu)
			desc.Memory = memory
			desc.Previous = table.DescID

			table.DescID = desc.ID
			table.Desc = &desc

			err = svc.so.SetServiceDesc(table)
			if err == nil {
				svc.svc = &table
			}

			return err
		}
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceResourceUpdating, statusServiceResourceUpdated, statusServiceResourceUpdateFailed)

	return sl.Run(isnotInProgress, update, false)
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
		type pending struct {
			u   *unit
			eng *cluster.Engine
			add []structs.VolumeRequire
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
				return errors.Errorf("")
			}

			add, err := u.prepareExpandVolume(target)
			if err != nil {
				return err
			}

			err = actor.IsNodeStoreEnough(eng, add)
			if err != nil {
				return err
			}

			pendings = append(pendings, pending{
				u:   u,
				eng: eng,
				add: add,
			})
		}

		// expand volume size
		for _, pu := range pendings {
			err := actor.ExpandVolumes(pu.eng, pu.u.u.ID, pu.add)
			if err != nil {
				return err
			}
		}

		{
			// update Service.Desc
			table, err := svc.so.GetService(svc.svc.ID)
			if err != nil {
				return err
			}

			var old []structs.VolumeRequire
			r := strings.NewReader(table.Desc.Volumes)
			err = json.NewDecoder(r).Decode(&old)
			if err != nil {
				old = []structs.VolumeRequire{}
			}

			out := mergeVolumeRequire(old, target)
			vb, err := json.Marshal(out)
			if err != nil {
				return errors.WithStack(err)
			}

			desc := *table.Desc
			desc.ID = utils.Generate32UUID()
			desc.Volumes = string(vb)
			desc.Previous = table.DescID

			table.DescID = desc.ID
			table.Desc = &desc

			return svc.so.SetServiceDesc(table)
		}

	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, nil,
		statusServiceVolumeExpanding, statusServiceVolumeExpanded, statusServiceVolumeExpandFailed)

	return sl.Run(isnotInProgress, expansion, false)
}

func mergeVolumeRequire(old, update []structs.VolumeRequire) []structs.VolumeRequire {
	if len(old) == 0 {
		return update
	}

	out := make([]structs.VolumeRequire, 0, len(old))

	for i := range old {
		found := false

	loop:
		for v := range update {
			if old[i].Name == update[v].Name && old[i].Type == update[v].Type {
				out = append(out, update[v])
				found = true
				break loop
			}
		}

		if !found {
			out = append(out, old[i])
		}
	}

	return out
}
