package mesos

import "github.com/docker/swarm/cluster"

// EngineByAddr is exported,not implement yet
func (c *Cluster) EngineByAddr(addr string) *cluster.Engine {
	return nil
}

// Engine is exported,not implement yet
func (c *Cluster) Engine(IDOrName string) *cluster.Engine {
	return nil
}

// ListEngines is exported,not implement yet
func (c *Cluster) ListEngines(list ...string) map[string]*cluster.Engine {
	return nil
}

// AddPendingContainer is exported,not implement yet
func (c *Cluster) AddPendingContainer(name, swarmID, engineID string, config *cluster.ContainerConfig) error {
	return nil
}

// RemovePendingContainer is exported,not implement yet
func (c *Cluster) RemovePendingContainer(swarmID ...string) {}
