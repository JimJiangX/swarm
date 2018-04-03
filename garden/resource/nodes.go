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

type nodeOrmer interface {
	lister
	database.NodeOrmer
}

type hostManager struct {
	ec    engineCluster
	dco   nodeOrmer
	nodes []nodeWithTask
}

// NewHostManager returns host manager
func NewHostManager(dco nodeOrmer, ec engineCluster, nodes []nodeWithTask) hostManager {
	return hostManager{
		dco:   dco,
		ec:    ec,
		nodes: nodes,
	}
}

func (m hostManager) getCluster(ID string) (database.Cluster, error) {

	return m.dco.GetCluster(ID)
}

// RemoveCluster remove cluster in db,check if there are none of nodes related to the Cluster.
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
