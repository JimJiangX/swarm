package swarm

import (
	"github.com/docker/swarm/cluster"
)

func (c *Cluster) EngineByAddr(addr string) *cluster.Engine {
	return c.getEngineByAddr(addr)
}
