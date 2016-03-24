package swarm

import (
	"errors"
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/scheduler/node"
)

func (region *Region) ServiceToScheduler(svc *Service) {
	region.serviceSchedulerCh <- svc
}

func (region *Region) serviceScheduler() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%s", r, err)
		}

		log.Fatal("Service Scheduler Exit,%s", err)
	}()

	for {
		svc := <-region.serviceSchedulerCh

		if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
			continue
		}

		svc.Lock()

		resourceAlloc := make([]*preAllocResource, 0, len(svc.base.Modules))

		for _, module := range svc.base.Modules {
			// query image from database
			if module.Config.Image == "" {
				image, err := region.GetImage(module.Name, module.Version)
				if err != nil {
					goto failure
				}

				module.Config.Image = image.ImageID
			}

			config := cluster.BuildContainerConfig(module.Config.ContainerConfig)
			err = validateContainerConfig(config)
			if err != nil {
				goto failure
			}

			storeType, storeSize := module.Store()
			filters := region.listShortIdleStore(storeType, int64(module.Num)*storeSize)
			list := region.listCandidateNodes(module.Nodes, module.Type, filters...)

			preAlloc, err := region.BuildPendingContainers(list, module.Type, module.Num, config, false)

			resourceAlloc = append(resourceAlloc, preAlloc...)

			if err != nil {
				goto failure
			}

		}

		for i := range resourceAlloc {
			svc.units = append(svc.units, resourceAlloc[i].unit)
			svc.pendingContainers[resourceAlloc[i].swarmID] = resourceAlloc[i].pendingContainer
		}

		// scheduler success
		atomic.StoreInt64(&svc.Status, 1)

		svc.Unlock()

		region.ServiceToExecute(svc)
		continue

	failure:

		err = region.Recycle(resourceAlloc)

		// scheduler failed
		for i := range resourceAlloc {
			region.scheduler.Lock()
			delete(region.pendingContainers, resourceAlloc[i].swarmID)
			region.scheduler.Unlock()
		}

		svc.pendingContainers = make(map[string]*pendingContainer)
		svc.units = make([]*unit, 0, 10)

		atomic.StoreInt64(&svc.Status, 10)

		svc.Unlock()
	}

	return err
}

func (region *Region) BuildPendingContainers(list []*node.Node, Type string, num int,
	config *cluster.ContainerConfig, withImageAffinity bool) ([]*preAllocResource, error) {

	region.scheduler.Lock()
	defer region.scheduler.Unlock()

	candidates, err := region.Scheduler(list, config, num, withImageAffinity)
	if err != nil {

		var retries int64
		//  fails with image not found, then try to reschedule with image affinity
		bImageNotFoundError, _ := regexp.MatchString(`image \S* not found`, err.Error())
		if bImageNotFoundError && !config.HaveNodeConstraint() {
			// Check if the image exists in the cluster
			// If exists, retry with a image affinity
			if region.Image(config.Image) != nil {
				candidates, err = region.Scheduler(list, config, num, true)
				retries++
			}
		}

		for ; retries < region.createRetry && err != nil; retries++ {
			log.WithFields(log.Fields{"Name": "Swarm"}).Warnf("Failed to scheduler: %s, retrying", err)
			candidates, err = region.Scheduler(list, config, num, true)
		}
	}

	if err != nil {
		return nil, err
	}

	if len(candidates) < num {
		return nil, errors.New("Not Enough Nodes")
	}

	preAllocs, err := region.pendingAlloc(candidates[0:num], Type, config)

	region.scheduler.Unlock()

	return preAllocs, err
}

func (region *Region) pendingAlloc(candidates []*node.Node, Type string,
	templConfig *cluster.ContainerConfig) ([]*preAllocResource, error) {

	allocs := make([]*preAllocResource, 0, 5)

	for i := range candidates {
		preAlloc := newPreAllocResource()
		allocs = append(allocs, preAlloc)

		id := region.generateUniqueID()
		engine, ok := region.engines[candidates[i].ID]
		if !ok {
			return allocs, errors.New("Engine Not Found")
		}

		image, err := region.getImageByID(templConfig.Image)
		if err != nil {
			return allocs, err
		}

		ports, err := database.SelectAvailablePorts(len(image.PortSlice))
		if err != nil {
			return allocs, err
		}

		for i := range ports {
			ports[i].Name = image.PortSlice[i].Name
			ports[i].UnitID = id
			ports[i].Proto = image.PortSlice[i].Proto
			ports[i].Allocated = true

		}

		preAlloc.unit = &unit{
			Unit: database.Unit{
				ID: id,
				// Name:      string(id[:8]) + "_",
				Type:      Type,
				ImageID:   image.ID,
				ImageName: image.Name + "_" + image.Version,
				NodeID:    engine.ID,
				Status:    0,
				CreatedAt: time.Now(),
			},
			configures: image.KeySets,
			ports:      ports,
		}

		config, err := region.allocResource(preAlloc, engine, *templConfig, Type)
		if err != nil {
			return allocs, fmt.Errorf("error resource alloc")

		}

		swarmID := config.SwarmID()
		if swarmID == "" {
			// Associate a Swarm ID to the container we are creating.
			swarmID = region.generateUniqueID()
			config.SetSwarmID(swarmID)
		}

		preAlloc.swarmID = swarmID
		preAlloc.pendingContainer = &pendingContainer{
			Name:   id,
			Config: config,
			Engine: engine,
		}
		region.pendingContainers[swarmID] = preAlloc.pendingContainer

		err = preAlloc.consistency()
		if err != nil {
			return allocs, err
		}
	}

	return allocs, nil
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

	return region.SelectNodeByCluster(nodes, num, true)
}

// listCandidateNodes returns all validated engines in the cluster, excluding pendingEngines.
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

func (r *Region) SelectNodeByCluster(nodes []*node.Node, num int, diff bool) ([]*node.Node, error) {
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
