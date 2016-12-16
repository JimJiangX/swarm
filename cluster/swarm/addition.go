package swarm

import (
	"errors"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
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

// ListNodes returns all validated engines in the cluster, excluding pendingEngines.
// list all engines when len(list) == 0.
func (c *Cluster) ListNodes(list ...string) []*node.Node {
	c.RLock()
	defer c.RUnlock()

	all := true
	if len(list) > 0 {
		all = false
	}

	out := make([]*node.Node, 0, len(c.engines))
loop:
	for _, e := range c.engines {

		if !all {
			for i := range list {
				if list[i] == e.ID || list[i] == e.Name {
					continue loop
				}
			}
		}

		node := node.NewNode(e)
		for _, pc := range c.pendingContainers {
			if pc.Engine.ID == e.ID && node.Container(pc.Config.SwarmID()) == nil {
				node.AddContainer(pc.ToContainer())
			}
		}
		out = append(out, node)
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

func (c *Cluster) RemovePendingContainer(swarmID string) {
	c.scheduler.Lock()

	delete(c.pendingContainers, swarmID)

	c.scheduler.Unlock()
}
