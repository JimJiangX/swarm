package garden

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type unit struct {
	u       database.Unit
	uo      database.UnitOrmer
	cluster cluster.Cluster
}

//func (u unit) getNetworking() ([]Networking, error) {
//	return nil, nil
//}

func (u unit) getContainer() *cluster.Container {
	if u.u.ContainerID != "" {
		return u.cluster.Container(u.u.ContainerID)
	}

	return nil
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
