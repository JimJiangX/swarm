package resource

import (
	"github.com/docker/swarm/garden/database"
)

type hostManager struct {
	ec  engineCluster
	dco database.NodeOrmer
}

func NewHostManager(dco database.NodeOrmer, ec engineCluster) hostManager {
	return hostManager{
		dco: dco,
		ec:  ec,
	}
}

func (m hostManager) getCluster(ID string) (database.Cluster, error) {

	return m.dco.GetCluster(ID)
}

func (m hostManager) RemoveCluster(ID string) error {
	cl, err := m.getCluster(ID)
	if err != nil && database.IsNotFound(err) {
		return nil
	}

	return m.dco.DelCluster(cl.ID)
}
