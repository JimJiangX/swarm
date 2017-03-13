package swarm

import (
	"errors"

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

// ListEngines returns all the engines in the cluster.
// This is for reporting, scheduling, hence pendingEngines are included.
// containers in pendingContainers are include.
func (c *Cluster) ListEngines(list ...string) []*cluster.Engine {
	c.RLock()
	defer c.RUnlock()

	all := true
	if len(list) > 0 {
		all = false
	}

	out := make([]*cluster.Engine, 0, len(c.engines)+len(c.pendingEngines))

engines:
	for _, n := range c.engines {
		if !all {
			for i := range list {
				if list[i] == n.ID || list[i] == n.Name {
					continue engines
				}
			}
		}

		for _, pc := range c.pendingContainers {
			if pc.Engine.ID == n.ID && n.Containers().Get(pc.Config.SwarmID()) == nil {
				n.AddContainer(pc.ToContainer())
			}
		}

		out = append(out, n)
	}

pendingEngines:
	for _, n := range c.pendingEngines {
		if !all {
			for i := range list {
				if list[i] == n.ID || list[i] == n.Name {
					continue pendingEngines
				}
			}
		}

		for _, pc := range c.pendingContainers {
			if pc.Engine.ID == n.ID && n.Containers().Get(pc.Config.SwarmID()) == nil {
				n.AddContainer(pc.ToContainer())
			}
		}

		out = append(out, n)
	}

	return out
}

func (c *Cluster) AddPendingContainer(name, swarmID, engineID string, config *cluster.ContainerConfig) error {
	e := c.Engine(engineID)
	if e == nil {
		return errors.New("not found engine")
	}

	c.scheduler.Lock()

	c.pendingContainers[swarmID] = &pendingContainer{
		Name:   name,
		Config: config,
		Engine: e,
	}

	c.scheduler.Unlock()

	return nil
}

func (c *Cluster) RemovePendingContainer(swarmID ...string) {
	c.scheduler.Lock()

	for i := range swarmID {
		delete(c.pendingContainers, swarmID[i])
	}

	c.scheduler.Unlock()
}

// deleteEngine if engine is not healthy
func (c *Cluster) deleteEngine(addr string) bool {
	engine := c.getEngineByAddr(addr)
	if engine == nil {
		return false
	}

	// check engine whether healthy
	err := engine.RefreshNetworks()
	if err == nil && engine.IsHealthy() {
		return false
	}

	return c.removeEngine(addr)
}
