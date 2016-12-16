package mesos

import "github.com/docker/swarm/cluster"

func (c *Cluster) EngineByAddr(addr string) *cluster.Engine {
	return nil
}

func (c *Cluster) Engine(IDOrName string) *cluster.Engine {
	return nil
}

func (c *Cluster) AddPendingContainer(name, swarmID, engineID string, config *cluster.ContainerConfig) error {
	return nil
}

func (c *Cluster) RemovePendingContainer(swarmID string) {}
