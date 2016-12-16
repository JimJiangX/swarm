package garden

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"golang.org/x/net/context"
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

func (u unit) startContainer() error {
	engine := u.getEngine()
	if engine == nil {
		return nil
	}

	if u.u.ContainerID != "" {
		return engine.StartContainer(u.u.ContainerID, nil)
	}

	return engine.StartContainer(u.u.Name, nil)
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
