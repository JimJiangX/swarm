package resource

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type nodes struct {
	clsuter cluster.Cluster
	dco     database.NodeOrmer
}

func NewNodes(dco database.NodeOrmer, c cluster.Cluster) *nodes {
	return &nodes{
		dco:     dco,
		clsuter: c,
	}
}

func (ns *nodes) AddCluster(c database.Cluster) error {
	if c.ID == "" {
		return nil
	}

	return ns.dco.InsertCluster(c)
}

func (ns *nodes) getCluster(nameOrID string) (database.Cluster, error) {

	return ns.dco.GetCluster(nameOrID)
}

func (ns *nodes) RemoveCluster(nameOrID string) error {
	cl, err := ns.getCluster(nameOrID)
	if err != nil && database.IsNotFound(err) {
		return nil
	}

	err = ns.dco.DelCluster(cl.ID)
	if err != nil {
		return err
	}

	return nil
}
