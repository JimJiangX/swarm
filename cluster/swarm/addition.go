package swarm

import (
	"github.com/docker/swarm/cluster"
)

func (c *Cluster) EngineByAddr(addr string) *cluster.Engine {
	return c.getEngineByAddr(addr)
}

func (c *Cluster) Engine(IDOrName string) *cluster.Engine {
	c.RLock()
	defer c.RUnlock()

	for _, engine := range c.engines {
		if engine.ID == IDOrName || engine.Name == IDOrName {
			return engine
		}
	}

	for _, engine := range c.pendingEngines {
		if engine.ID == IDOrName || engine.Name == IDOrName {
			return engine
		}
	}

	return nil
}
