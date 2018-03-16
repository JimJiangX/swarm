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
func (c *Cluster) ListEngines(list ...string) []*cluster.Engine {
	c.RLock()

	all := true
	if len(list) > 0 {
		all = false
	}

	out := make([]*cluster.Engine, 0, len(c.engines))

	for _, n := range c.engines {
		if !all {
			found := false
		engines:
			for i := range list {
				if list[i] == n.ID || list[i] == n.Name {
					found = true
					break engines
				}
			}

			if !found {
				continue
			}
		}

		for _, pc := range c.pendingContainers {
			if pc.Engine.ID == n.ID {
				n.AddContainer(pc.ToContainer())
			}
		}

		out = append(out, n)
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
