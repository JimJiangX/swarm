package resource

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type Datacenter struct {
	clsuter cluster.Cluster
	dco     database.ClusterOrmer
}

func NewDatacenter(dco database.ClusterOrmer, c cluster.Cluster) *Datacenter {
	return &Datacenter{
		dco:     dco,
		clsuter: c,
	}
}

func (dc *Datacenter) AddCluster(c database.Cluster) error {
	if c.ID == "" {
		return nil
	}

	_, err := dc.getCluster(c.ID)
	if err == nil {
		return nil
	}

	err = dc.dco.InsertCluster(c)
	if err != nil {
		return err
	}

	return nil
}

func (dc *Datacenter) getCluster(nameOrID string) (database.Cluster, error) {

	return dc.dco.GetCluster(nameOrID)
}

func (dc *Datacenter) RemoveCluster(nameOrID string) error {
	cl, err := dc.getCluster(nameOrID)
	if err != nil && database.IsNotFound(err) {
		return nil
	}

	err = dc.dco.DeleteCluster(cl.ID)
	if err != nil {
		return err
	}

	return nil
}
