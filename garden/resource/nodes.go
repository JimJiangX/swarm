package resource

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/pkg/errors"
)

type engineCluster interface {
	Engine(IDOrName string) *cluster.Engine
	EngineByAddr(addr string) *cluster.Engine
}

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

	n, err := m.dco.CountNodeByCluster(cl.ID)
	if err != nil {
		return err
	}

	if n > 0 {
		return errors.Errorf("%d nodes belongs to Cluster:%s", n, cl.ID)
	}

	return m.dco.DelCluster(cl.ID)
}
