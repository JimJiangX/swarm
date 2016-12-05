package resource

import (
	"context"
	"sync"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
)

type Datacenter struct {
	lock     *sync.RWMutex
	clusters map[string]database.Cluster
	nodes    map[string]database.Node

	clsuter cluster.Cluster
	dco     database.ClusterOrmer
}

func NewDatacenter(dco database.ClusterOrmer, c cluster.Cluster) *Datacenter {
	return &Datacenter{
		lock:     new(sync.RWMutex),
		clusters: make(map[string]database.Cluster, 10),
		nodes:    make(map[string]database.Node, 50),
		dco:      dco,
		clsuter:  c,
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

	dc.lock.Lock()
	dc.clusters[c.ID] = c
	dc.lock.Unlock()

	return nil
}

func (dc *Datacenter) getCluster(nameOrID string) (database.Cluster, error) {
	dc.lock.RLock()
	c, ok := dc.clusters[nameOrID]
	dc.lock.RUnlock()

	if ok {
		return c, nil
	}

	c, err := dc.dco.GetCluster(nameOrID)
	if err != nil {
		return c, err
	}

	dc.lock.Lock()
	dc.clusters[c.ID] = c
	dc.lock.Unlock()

	return c, nil
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

	dc.lock.Lock()
	delete(dc.clusters, cl.ID)
	dc.lock.Unlock()

	return nil
}

func (dc *Datacenter) InstallNodes(ctx context.Context, list []nodeWithTask) error {
	for i := range list {
		_, err := dc.getCluster(list[i].Node.ClusterID)
		if err != nil {
			return err
		}
	}

	nodes := make([]database.Node, len(list))
	tasks := make([]database.Task, len(list))

	for i := range list {
		nodes[i] = list[i].Node
		tasks[i] = list[i].Task
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	err := dc.dco.InsertNodesAndTask(nodes, tasks)
	if err != nil {
		return err
	}

	dc.lock.Lock()
	for i := range nodes {
		dc.nodes[nodes[i].ID] = nodes[i]
	}
	dc.lock.Unlock()

	config, err := dc.dco.GetSysConfig()
	if err != nil {
		return err
	}

	// TODO:go func()error
	for i := range list {
		err := list[i].distribute(ctx, dc.dco, config)
		if err != nil {
			// TODO:
		}
	}

	go dc.registerNodes(ctx, list, config)

	return nil
}
