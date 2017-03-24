package garden

import (
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (svc *Service) UpdateImage(ctx context.Context, kvc kvstore.Client,
	im database.Image, task database.Task, authConfig *types.AuthConfig) error {

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

		{
			table, err := svc.so.GetService(svc.spec.ID)
			if err != nil {
				return err
			}
			desc := *table.Desc
			desc.ID = utils.Generate32UUID()
			desc.Image = im.Version()
			desc.ImageID = im.ImageID

			table.DescID = desc.ID
			table.Desc = &desc

			err = svc.so.SetServiceDesc(table)

			return err
		}
	}

	tl := tasklock.NewServiceTask(svc.spec.ID, svc.so, &task,
		statusServiceImageUpdating,
		statusServiceImageUpdated,
		statusServiceImageUpdateFailed)

	return tl.Run(isnotInProgress, update)
}

func (svc *Service) ServiceUpdate(ctx context.Context, actor allocator, ncpu, memory int64, task database.Task) error {
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

			cpuset, err := actor.AlloctCPUMemory(c.Config, node, ncpu-n, memory, nil)
			if err != nil {
				return err
			}
			if pu.cpuset == "" {
				pu.cpuset = cpuset
			} else {
				pu.cpuset = pu.cpuset + "," + cpuset
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

	return nil
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
