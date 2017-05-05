package garden

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (gd *Garden) ServiceMigrate(ctx context.Context, svc *Service, nameOrID string, candidates []string, async bool) (string, error) {
	migrate := func() error {
		opts, err := svc.getScheduleOption()
		if err != nil {
			return err
		}
		if len(candidates) > 0 {
			opts.Nodes.Constraints = []string{nodeLabel + "==" + strings.Join(candidates, "|")}
		}

		u, err := svc.getUnit(nameOrID)
		if err != nil {
			return err
		}

		oldEngine := u.getEngine()

		cmds, err := svc.generateUnitsCmd(ctx)
		if err != nil {
			return err
		}

		err = u.stopService(ctx, cmds.GetCmd(u.u.ID, structs.StopServiceCmd), true)
		if err != nil {
			return err
		}

		defer func(u unit) {
			if err != nil {
				_err := u.startService(ctx, cmds.GetCmd(u.u.ID, structs.StartServiceCmd))
				if _err != nil {
					err = fmt.Errorf("%+v\n%+v", err, _err)
				}
			}
		}(*u)

		old := u.getContainer()
		if old == nil {
			old, err = getContainerFromKV(gd.kvClient, u.u.ContainerID)
			if err != nil {
				return err
			}
		}

		old.Config.HostConfig.CpusetCpus = strconv.Itoa(opts.Require.Require.CPU)
		actor := alloc.NewAllocator(gd.ormer, gd.Cluster)
		swarmID := utils.Generate32UUID()

		gd.Lock()
		defer gd.Unlock()

		gd.scheduler.Lock()
		nodes, err := gd.schedule(ctx, actor, old.Config, opts)
		if err != nil {
			gd.scheduler.Unlock()
			return err
		}

		gd.scheduler.Unlock()

		gd.AddPendingContainer(u.u.Name, swarmID, nodes[0].ID, old.Config)

		if len(nodes) < 1 {
			return errors.Errorf("not enough nodes for allocation,%d<%d", len(nodes), 1)
		}

		_, err = actor.AlloctCPUMemory(old.Config, nodes[0], int64(opts.Require.Require.CPU), opts.Require.Require.Memory, nil)
		if err != nil {
			return err
		}

		// TODO:alloc new networking

		lvs, err := u.getVolumesByUnit()
		if err != nil {
			return err
		}

		eng := gd.Cluster.EngineByAddr(nodes[0].Addr)
		if eng == nil {
			// TODO:
			return nil
		}

		err = actor.MigrateVolumes(u.u.ID, old.Config, oldEngine, eng, lvs)
		if err != nil {
			return err
		}

		auth, err := gd.AuthConfig()
		if err != nil {
			return err
		}

		container, err := eng.CreateContainer(old.Config, u.u.Name, true, auth)
		if err != nil {
			return err
		}

		u.u.ContainerID = container.ID
		u.u.EngineID = eng.ID

		err = u.startService(ctx, cmds.GetCmd(u.u.ID, structs.StartServiceCmd))
		if err != nil {
			return err
		}

		err = gd.Cluster.RemoveContainer(old, false, true)
		if err != nil {
			return err
		}

		// TODO: register service

		return nil
	}

	task := database.NewTask(svc.svc.Name, database.UnitMigrateTask, svc.svc.ID, nameOrID, nil, 300)

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &task,
		statusServiceUnitMigrating, statusServiceUnitMigrated, statusServiceUnitMigrateFailed)

	err := sl.Run(isnotInProgress, migrate, async)

	return task.ID, err
}
