package garden

import (
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"golang.org/x/net/context"
)

const (
	statusUnitNoContent = iota

	statusUnitAllocting
	statusUnitAllocted
	statusUnitAlloctionFailed

	statusUnitCreating
	statusUnitCreated
	statusUnitCreateFailed

	statusUnitStarting // start contaier and start service
	statusUnitStarted
	statusUnitStartFailed

	statusUnitStoping
	statusUnitStoped
	statusUnitStopFailed

	statusUnitMigrating
	statusUnitMigrated
	statusUnitMigrateFailed

	statusUnitRebuilding
	statusUnitRebuilt
	statusUnitRebuildFailed

	statusUnitDeleting

	statusUnitBackuping
	statusUnitBackuped
	statusUnitBackupFailed

	statusUnitRestoring
	statusUnitRestored
	statusUnitRestoreFailed

	statusContainerCreated
	statusContainerRunning
	statusContainerPaused
	statusContainerRestarted
	statusContainerDead
	statusContainerExited
)

type unit struct {
	u       database.Unit
	uo      database.UnitOrmer
	cluster cluster.Cluster
}

func newUnit(u database.Unit, uo database.UnitOrmer, cluster cluster.Cluster) *unit {
	return &unit{
		u:       u,
		uo:      uo,
		cluster: cluster,
	}
}

//func (u unit) getNetworking() ([]Networking, error) {
//	return nil, nil
//}

func (u unit) getContainer() *cluster.Container {
	if u.u.ContainerID != "" {
		return u.cluster.Container(u.u.ContainerID)
	}

	return u.cluster.Container(u.u.Name)
}

func (u unit) getVolumesByUnit() ([]database.Volume, error) {
	lvs, err := u.uo.ListVolumesByUnitID(u.u.ID)

	return lvs, err
}

func (u unit) getEngine() *cluster.Engine {
	if u.u.EngineID != "" {
		return u.cluster.Engine(u.u.EngineID)
	}

	c := u.getContainer()
	if c != nil {
		return c.Engine
	}

	return nil
}

func (u unit) startContainer(ctx context.Context) error {
	engine := u.getEngine()
	if engine == nil {
		return nil
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	if u.u.ContainerID != "" {
		return engine.StartContainer(u.u.ContainerID, nil)
	}

	return engine.StartContainer(u.u.Name, nil)
}

func (u unit) stopContainer(ctx context.Context) error {
	engine := u.getEngine()
	if engine == nil {
		return nil
	}

	client := engine.ContainerAPIClient()
	if client == nil {
		return nil
	}

	timeout := 30 * time.Second
	err := client.ContainerStop(ctx, u.u.Name, &timeout)
	engine.CheckConnectionErr(err)

	return err
}

func (u unit) removeContainer(ctx context.Context) error {
	c := u.getContainer()
	if c == nil {
		return nil
	}

	err := c.Engine.RemoveContainer(c, true, false)
	c.Engine.CheckConnectionErr(err)

	return err
}

func (u unit) removeVolumes(ctx context.Context) error {
	lvs, err := u.getVolumesByUnit()
	if err != nil {
		return err
	}

	engine := u.getEngine()
	if err != nil {
		return nil
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	for i := range lvs {
		err := engine.RemoveVolume(lvs[i].Name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u unit) containerExec(ctx context.Context, cmd []string) (types.ContainerExecInspect, error) {
	c := u.getContainer()
	if c != nil {
		return types.ContainerExecInspect{}, nil
	}

	if !c.Info.State.Running {
		return types.ContainerExecInspect{}, nil
	}

	return c.Exec(ctx, cmd)
}
