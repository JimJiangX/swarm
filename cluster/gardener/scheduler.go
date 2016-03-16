package gardener

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
)

func (c *Cluster) ServiceToScheduler(svc *Service) {
	c.serviceSchedulerCh <- svc
}

func (c *Cluster) ServiceToExecute(svc *Service) {
	c.serviceExecuteCh <- svc
}

func (c *Cluster) ServiceScheduler() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%s", r, err)
		}
	}()

	for {
		svc := <-c.serviceSchedulerCh

		if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
			continue
		}

		// datacenter scheduler

		// run scheduler on selected datacenter

		// container created on selected engine

		// modify scheduler label

		c.ServiceToExecute(svc)
	}

	return err
}

func (c *Cluster) buildPendingContainers(list []string, defConfig *cluster.ContainerConfig, num int, withImageAffinity bool) ([]*pendingContainer, error) {
	swarmID := defConfig.SwarmID()
	if swarmID != "" {
		return nil, errors.New("Swarm ID to the container have created")
	}

	c.scheduler.Lock()

	candidates, err := c.Scheduler(list, defConfig, num, false)
	if err != nil {

		candidates, err = c.Scheduler(list, defConfig, num, true)
		if err != nil {
			c.scheduler.Unlock()
			return nil, err
		}
	}

	if len(candidates) < num {
		c.scheduler.Unlock()
		return nil, errors.New("Not Enough Nodes")
	}

	pendingContainers := make(map[string]*pendingContainer, num)

	for i := range candidates {

		engine, ok := c.engines[candidates[i].ID]
		if !ok {
			c.scheduler.Unlock()
			return nil, fmt.Errorf("error creating container")
		}

		config := *defConfig

		swarmID := config.SwarmID()
		if swarmID == "" {
			// Associate a Swarm ID to the container we are creating.
			swarmID = c.generateUniqueID()
			config.SetSwarmID(swarmID)
		}

		pendingContainers[swarmID] = &pendingContainer{
			Name:   c.generateUniqueID(),
			Config: &config,
			Engine: engine,
		}
	}

	for key, value := range pendingContainers {
		c.pendingContainers[key] = value
	}

	c.scheduler.Unlock()

	return nil, nil
}

func (c *Cluster) Scheduler(list []string, config *cluster.ContainerConfig, num int, withImageAffinity bool) ([]*node.Node, error) {

	if network := c.Networks().Get(config.HostConfig.NetworkMode); network != nil && network.Scope == "local" {
		if !config.HaveNodeConstraint() {
			config.AddConstraint("node==~" + network.Engine.Name)
		}
		config.HostConfig.NetworkMode = network.Name
	}

	if withImageAffinity {
		config.AddAffinity("image==" + config.Image)
	}

	nodes, err := c.scheduler.SelectNodesForContainer(c.UPMlistNodes(list), config)

	if withImageAffinity {
		config.RemoveAffinity("image==" + config.Image)
	}

	if err != nil {
		return nil, err
	}

	return c.selectNodeByCluster(nodes, num, true)
}

// UPMlistNodes returns all validated engines in the cluster, excluding pendingEngines.
func (c *Cluster) UPMlistNodes(names []string) []*node.Node {
	c.RLock()
	defer c.RUnlock()

	out := make([]*node.Node, 0, len(c.engines))

	if len(names) == 0 {

		for _, e := range c.engines {
			node := node.NewNode(e)

			for _, c := range c.pendingContainers {
				if c.Engine.ID == e.ID && node.Container(c.Config.SwarmID()) == nil {
					node.AddContainer(c.ToContainer())
				}
			}

			out = append(out, node)
		}

	} else {

		for _, name := range names {

			if e, ok := c.engines[name]; ok {
				node := node.NewNode(e)

				for _, c := range c.pendingContainers {
					if c.Engine.ID == e.ID && node.Container(c.Config.SwarmID()) == nil {
						node.AddContainer(c.ToContainer())
					}
				}

				out = append(out, node)

			}
		}
	}

	return out
}

func (c *Cluster) selectNodeByCluster(nodes []*node.Node, num int, diff bool) ([]*node.Node, error) {

	if len(nodes) < num {
		return nil, errors.New("Not Enough Nodes")

	}

	if !diff {
		return nodes[0:num:num], nil
	}

	m := make(map[string][]*node.Node)

	for i := range nodes {

		dc, err := c.DatacenterByNode(nodes[i].ID)
		if err != nil {
			continue
		}

		if s, ok := m[dc.ID]; ok {

			m[dc.ID] = append(s, nodes[i])

		} else {

			m[dc.ID] = []*node.Node{nodes[i]}

		}
	}

	if diff && len(m) < 2 {
		return nil, errors.New("Not Match")
	}

	candidates := make([]*node.Node, num)

	// select nodes
	seq := 0

	if len(m) >= num {
		for _, v := range m {
			candidates[seq] = v[0]

			seq++
			if seq >= num {
				return candidates, nil
			}
		}

	} else {

		count := make(map[string]int)

		for i := range nodes {

			dc, err := c.DatacenterByNode(nodes[i].ID)
			if err != nil {
				continue
			}

			if count[dc.ID] < num/2 {

				candidates[seq] = nodes[i]

				count[dc.ID]++
				seq++
				if seq >= num {
					return candidates, nil
				}

			}
		}

	}

	return nil, errors.New("Not Match")
}

func (c *Cluster) ServiceExecute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%s", r, err)
		}
	}()

	for {
		svc := <-c.serviceExecuteCh

		if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
			continue
		}
	}

	return err
}
