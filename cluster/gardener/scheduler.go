package gardener

import (
	"errors"
	"fmt"
	"regexp"
	"sync/atomic"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/gardener/database"
	"github.com/docker/swarm/scheduler/node"
)

func (region *Region) ServiceToScheduler(svc *Service) {
	region.serviceSchedulerCh <- svc
}

func (region *Region) ServiceScheduler() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%s", r, err)
		}
	}()

	for {
		svc := <-region.serviceSchedulerCh

		if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
			continue
		}

		svc.Lock()
		// datacenter scheduler

		// run scheduler on selected datacenter

		// container created on selected engine

		// modify scheduler label

		for _, module := range svc.base.Modules {
			// query image from database
			if module.Config.Image == "" {
				image, err := database.QueryImage(module.Name, module.Version)
				if err != nil {
					goto failure
				}

				module.Config.Image = image.ID
			}

			config := cluster.BuildContainerConfig(module.Config.ContainerConfig)
			err = validateContainerConfig(config)
			if err != nil {
				goto failure
			}

			storeType, storeSize := module.Store()
			filters := region.listShortIdleStore(storeType, int64(module.Num)*storeSize)
			list := region.listCandidateNodes(module.Nodes, module.Type, filters...)

			pendingContainers, err := region.buildPendingContainers(list, module.Type, module.Num, config, false)
			if err != nil {
				goto failure
			}

			for key, value := range pendingContainers {
				svc.pendingContainers[key] = value
			}

		}

		// scheduler success
		atomic.StoreInt64(&svc.Status, 1)

		svc.Unlock()

		region.ServiceToExecute(svc)
		continue

	failure:

		// scheduler failed
		for swarmID := range svc.pendingContainers {

			region.scheduler.Lock()
			delete(region.pendingContainers, swarmID)
			region.scheduler.Unlock()

		}

		svc.pendingContainers = make(map[string]*pendingContainer)
		atomic.StoreInt64(&svc.Status, 10)

		svc.Unlock()

	}

	return err
}

func (region *Region) buildPendingContainers(
	list []*node.Node, Type string, num int,
	templConfig *cluster.ContainerConfig,
	withImageAffinity bool) (map[string]*pendingContainer, error) {

	region.scheduler.Lock()
	defer region.scheduler.Unlock()

	candidates, err := region.Scheduler(list, templConfig, num, withImageAffinity)
	if err != nil {

		var retries int64
		//  fails with image not found, then try to reschedule with image affinity
		bImageNotFoundError, _ := regexp.MatchString(`image \S* not found`, err.Error())
		if bImageNotFoundError && !templConfig.HaveNodeConstraint() {
			// Check if the image exists in the cluster
			// If exists, retry with a image affinity
			if region.Image(templConfig.Image) != nil {
				candidates, err = region.Scheduler(list, templConfig, num, true)
				retries++
			}
		}

		for ; retries < region.createRetry && err != nil; retries++ {
			log.WithFields(log.Fields{"Name": "Swarm"}).Warnf("Failed to scheduler: %s, retrying", err)
			candidates, err = region.Scheduler(list, templConfig, num, true)
		}
	}

	if err != nil {
		return nil, err
	}

	if len(candidates) < num {
		return nil, errors.New("Not Enough Nodes")
	}

	pendingContainers := make(map[string]*pendingContainer, num)
	preAlloc := newPreAllocResource()

	for i := range candidates {

		engine, ok := region.engines[candidates[i].ID]
		if !ok {
			err = errors.New("Engine Not Found")
			goto failure
		}

		config, err := region.allocResource(preAlloc, engine, *templConfig, Type)
		if err != nil {
			err = fmt.Errorf("error resource alloc")
			goto failure
		}

		swarmID := config.SwarmID()
		if swarmID == "" {
			// Associate a Swarm ID to the container we are creating.
			swarmID = region.generateUniqueID()
			config.SetSwarmID(swarmID)
		}

		pendingContainers[swarmID] = &pendingContainer{
			Name:   region.generateUniqueID(),
			Config: config,
			Engine: engine,
		}
	}

	for key, value := range pendingContainers {
		region.pendingContainers[key] = value
	}

	err = preAlloc.consistency()

failure:
	if err != nil {
		region.Recycle(preAlloc)
	}

	region.scheduler.Unlock()

	return pendingContainers, nil
}

func (region *Region) Scheduler(list []*node.Node, config *cluster.ContainerConfig, num int, withImageAffinity bool) ([]*node.Node, error) {

	if network := region.Networks().Get(config.HostConfig.NetworkMode); network != nil && network.Scope == "local" {
		if !config.HaveNodeConstraint() {
			config.AddConstraint("node==~" + network.Engine.Name)
		}
		config.HostConfig.NetworkMode = network.Name
	}

	if withImageAffinity {
		config.AddAffinity("image==" + config.Image)
	}

	nodes, err := region.scheduler.SelectNodesForContainer(list, config)

	if withImageAffinity {
		config.RemoveAffinity("image==" + config.Image)
	}

	if err != nil {
		return nil, err
	}

	return region.selectNodeByCluster(nodes, num, true)
}

// listNodes returns all validated engines in the cluster, excluding pendingEngines.
func (r *Region) listCandidateNodes(names []string, dcTag string, filters ...string) []*node.Node {
	r.RLock()
	defer r.RUnlock()

	out := make([]*node.Node, 0, len(r.engines))

	if len(names) == 0 {

		for i := range r.datacenters {

			if r.datacenters[i].Type != dcTag ||
				isStringExist(r.datacenters[i].ID, filters) {

				continue
			}

			list := r.datacenters[i].listNodeID()

			for _, id := range list {

				if isStringExist(id, filters) {
					continue
				}

				e, ok := r.engines[id]
				if !ok {
					continue
				}

				node := node.NewNode(e)

				for _, c := range r.pendingContainers {

					if c.Engine.ID == e.ID && node.Container(c.Config.SwarmID()) == nil {
						node.AddContainer(c.ToContainer())
					}
				}

				out = append(out, node)

			}
		}

	} else {

		for _, name := range names {

			if isStringExist(name, filters) {
				continue
			}

			e, ok := r.engines[name]
			if !ok {
				continue
			}

			node := node.NewNode(e)

			for _, c := range r.pendingContainers {
				if c.Engine.ID == e.ID && node.Container(c.Config.SwarmID()) == nil {
					node.AddContainer(c.ToContainer())
				}
			}

			out = append(out, node)
		}
	}

	return out
}

func isStringExist(s string, list []string) bool {
	for i := range list {

		if s == list[i] {
			return true
		}
	}

	return false
}

func (r *Region) selectNodeByCluster(nodes []*node.Node, num int, diff bool) ([]*node.Node, error) {

	if len(nodes) < num {
		return nil, errors.New("Not Enough Nodes")

	}

	if !diff {
		return nodes[0:num:num], nil
	}

	m := make(map[string][]*node.Node)

	for i := range nodes {

		dc, err := r.DatacenterByNode(nodes[i].ID)
		if err != nil {
			continue
		}

		if s, ok := m[dc.ID]; ok {

			m[dc.ID] = append(s, nodes[i])

		} else {

			m[dc.ID] = []*node.Node{nodes[i]}

		}
	}

	if diff && len(m) < 2 {
		return nil, errors.New("Not Match")
	}

	candidates := make([]*node.Node, num)

	// select nodes
	seq := 0

	if len(m) >= num {
		for _, v := range m {
			candidates[seq] = v[0]

			seq++
			if seq >= num {
				return candidates, nil
			}
		}

	} else {

		count := make(map[string]int)

		for i := range nodes {

			dc, err := r.DatacenterByNode(nodes[i].ID)
			if err != nil {
				continue
			}

			if count[dc.ID] < num/2 {

				candidates[seq] = nodes[i]

				count[dc.ID]++
				seq++
				if seq >= num {
					return candidates, nil
				}

			}
		}

	}

	return nil, errors.New("Not Match")
}
