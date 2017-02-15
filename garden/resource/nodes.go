package resource

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type master struct {
	clsuter cluster.Cluster
	dco     database.NodeOrmer
}

func NewMaster(dco database.NodeOrmer, c cluster.Cluster) master {
	return master{
		dco:     dco,
		clsuter: c,
	}
}

func (m master) AddCluster(c database.Cluster) error {
	if c.ID == "" {
		return nil
	}

	return m.dco.InsertCluster(c)
}

func (m master) getCluster(nameOrID string) (database.Cluster, error) {

	return m.dco.GetCluster(nameOrID)
}

func (m master) RemoveCluster(nameOrID string) error {
	cl, err := m.getCluster(nameOrID)
	if err != nil && database.IsNotFound(err) {
		return nil
	}

	err = m.dco.DelCluster(cl.ID)
	if err != nil {
		return err
	}

	return nil
}
