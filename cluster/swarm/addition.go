package swarm

import (
	"errors"

	"github.com/docker/swarm/cluster"
)

func (c *Cluster) EngineByAddr(addr string) *cluster.Engine {
	return c.getEngineByAddr(addr)
}

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
	defer c.RUnlock()

	all := true
	if len(list) > 0 {
		all = false
	}

	out := make([]*cluster.Engine, 0, len(c.engines)+len(c.pendingEngines))

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
			if pc.Engine.ID == n.ID && n.Containers().Get(pc.Config.SwarmID()) == nil {
				n.AddContainer(pc.ToContainer())
			}
		}

		out = append(out, n)
	}

	for _, n := range c.pendingEngines {
		if !all {
			found := false
		pendingEngines:
			for i := range list {
				if list[i] == n.ID || list[i] == n.Name {
					found = true
					break pendingEngines
				}
			}

			if !found {
				continue
			}
		}

		for _, pc := range c.pendingContainers {
			if pc.Engine.ID == n.ID && n.Containers().Get(pc.Config.SwarmID()) == nil {
				n.AddContainer(pc.ToContainer())
			}
		}

		out = append(out, n)
	}

	engines := make([]*cluster.Engine, 0, len(out))

	for i := range out {
		exist := false

		for _, e := range engines {
			if out[i].ID == e.ID {
				exist = true
				break
			}
		}

		if !exist {
			engines = append(engines, out[i])
		}
	}

	return engines
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
