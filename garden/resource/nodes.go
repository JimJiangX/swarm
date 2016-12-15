package resource

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
)

type nodes struct {
	clsuter  cluster.Cluster
	dco      database.ClusterOrmer
	kvClient kvstore.Client
}

func NewNodes(dco database.ClusterOrmer, c cluster.Cluster, client kvstore.Client) *nodes {
	return &nodes{
		dco:      dco,
		clsuter:  c,
		kvClient: client,
	}
}

func (ns *nodes) AddCluster(c database.Cluster) error {
	if c.ID == "" {
		return nil
	}

	_, err := ns.getCluster(c.ID)
	if err == nil {
		return nil
	}

	err = ns.dco.InsertCluster(c)
	if err != nil {
		return err
	}

	return nil
}

func (ns *nodes) getCluster(nameOrID string) (database.Cluster, error) {

	return ns.dco.GetCluster(nameOrID)
}

func (ns *nodes) RemoveCluster(nameOrID string) error {
	cl, err := ns.getCluster(nameOrID)
	if err != nil && database.IsNotFound(err) {
		return nil
	}

	err = ns.dco.DeleteCluster(cl.ID)
	if err != nil {
		return err
	}

	return nil
}
