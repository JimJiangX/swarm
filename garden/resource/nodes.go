package resource

import (
	"github.com/docker/swarm/garden/database"
)

type master struct {
	ec  engineCluster
	dco database.NodeOrmer
}

func NewMaster(dco database.NodeOrmer, ec engineCluster) master {
	return master{
		dco: dco,
		ec:  ec,
	}
}

//func (m master) AddCluster(c database.Cluster) error {
//	if c.ID == "" {
//		return nil
//	}

//	return m.dco.InsertCluster(c)
//}

func (m master) getCluster(ID string) (database.Cluster, error) {

	return m.dco.GetCluster(ID)
}

func (m master) RemoveCluster(ID string) error {
	cl, err := m.getCluster(ID)
	if err != nil && database.IsNotFound(err) {
		return nil
	}

	return m.dco.DelCluster(cl.ID)
}
