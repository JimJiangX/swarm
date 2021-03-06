package swarm

import (
	"errors"

	"github.com/docker/swarm/cluster"
)

// EngineByAddr returns  *Engine by addr
func (c *Cluster) EngineByAddr(addr string) *cluster.Engine {
	return c.getEngineByAddr(addr)
}

// Engine returns *Engine by ID or Name
func (c *Cluster) Engine(IDOrName string) *cluster.Engine {
	if IDOrName == "" {
		return nil
	}

	c.RLock()
	defer c.RUnlock()

	if e, ok := c.engines[IDOrName]; ok {
		return e
	}

	for _, e := range c.engines {
		if e.ID == IDOrName || e.Name == IDOrName {
			return e
		}
	}

	for _, e := range c.pendingEngines {
		if e.ID == IDOrName || e.Name == IDOrName {
			return e
		}
	}

	return nil
}

// ListEngines returns all the engines in the cluster.
// This is for reporting, scheduling, hence pendingEngines are included.
// containers in pendingContainers are include.
func (c *Cluster) ListEngines(list ...string) map[string]*cluster.Engine {
	c.RLock()

	if len(list) > 0 {
		out := make(map[string]*cluster.Engine, len(list))

		for i := range list {
			eng, ok := c.engines[list[i]]
			if ok {
				for swarmID, pc := range c.pendingContainers {
					if pc.Engine.ID == eng.ID {

						c := pc.ToContainer()
						if c.ID == "" {
							c.ID = swarmID
						}

						eng.AddContainer(c)
					}
				}

				out[eng.IP] = eng
			}
		}

		c.RUnlock()

		return out
	}

	out := make(map[string]*cluster.Engine, len(c.engines))

	for _, eng := range c.engines {

		for swarmID, pc := range c.pendingContainers {
			if pc.Engine.ID == eng.ID {
				c := pc.ToContainer()
				if c.ID == "" {
					c.ID = swarmID
				}

				eng.AddContainer(c)
			}
		}

		out[eng.IP] = eng
	}

	c.RUnlock()

	return out
}

// AddPendingContainer add a pending container to Cluster
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

// RemovePendingContainer remove slice of specified pending Container by swarmID
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
		return true
	}

	// check engine whether healthy
	if engine.IsHealthy() {
		_, err := engine.ServerVersion()
		if err == nil {
			return false
		}
	}

	return c.removeEngine(addr)
}
