package swarm

import (
	"errors"
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/scheduler/node"
)

func (gd *Gardener) ServiceToScheduler(svc *Service) error {
	err := database.TxSetServiceStatus(&svc.Service, svc.task,
		_StatusServcieBuilding, _StatusTaskRunning, time.Time{}, "")
	if err != nil {
		return err
	}

	gd.serviceSchedulerCh <- svc

	return nil
}

func (gd *Gardener) serviceScheduler() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%v", r, err)
		}

		log.Fatal("Service Scheduler Exit,%s", err)
	}()

	for {
		svc := <-gd.serviceSchedulerCh

		if !atomic.CompareAndSwapInt64(&svc.Status, _StatusServcieBuilding, _StatusServiceAlloction) {
			continue
		}

		svc.Lock()

		resourceAlloc := make([]*preAllocResource, 0, len(svc.base.Modules))

		for i := range svc.base.Modules {

			preAlloc, err := gd.BuildPendingContainersPerModule(&svc.base.Modules[i])
			if len(preAlloc) > 0 {
				resourceAlloc = append(resourceAlloc, preAlloc...)
			}
			if err != nil {
				goto failure
			}
		}

		for i := range resourceAlloc {
			svc.units = append(svc.units, resourceAlloc[i].unit)
			svc.pendingContainers[resourceAlloc[i].swarmID] = resourceAlloc[i].pendingContainer
		}

		// scheduler success
		svc.Unlock()

		gd.ServiceToExecute(svc)
		continue

	failure:

		err = gd.Recycle(resourceAlloc)

		// scheduler failed
		for i := range resourceAlloc {
			gd.scheduler.Lock()
			delete(gd.pendingContainers, resourceAlloc[i].swarmID)
			gd.scheduler.Unlock()
		}

		svc.pendingContainers = make(map[string]*pendingContainer)
		svc.units = make([]*unit, 0, 10)

		svc.Service.SetServiceStatus(_StatusServiceAlloctionFailed, time.Now())

		svc.Unlock()
	}

	return err
}

func (gd *Gardener) BuildPendingContainersPerModule(module *structs.Module) ([]*preAllocResource, error) {
	// query image from database
	if module.Config.Image == "" {
		image, err := gd.GetImage(module.Name, module.Version)
		if err != nil {
			return nil, err
		}

		module.Config.Image = image.ImageID
	}

	config := cluster.BuildContainerConfig(module.Config, module.HostConfig, module.NetworkingConfig)
	err := validateContainerConfig(config)
	if err != nil {
		return nil, err
	}

	filters := gd.listShortIdleStore(module.Stores, module.Type, module.Num)
	list := gd.listCandidateNodes(module.Nodes, module.Type, filters...)

	return gd.BuildPendingContainers(list, module.Type, module.Num, module.Stores, config, false)
}

func (gd *Gardener) BuildPendingContainers(list []*node.Node, Type string, num int, stores []structs.DiskStorage,
	config *cluster.ContainerConfig, withImageAffinity bool) ([]*preAllocResource, error) {

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	candidates, err := gd.Scheduler(list, config, num, withImageAffinity)
	if err != nil {

		var retries int64
		//  fails with image not found, then try to reschedule with image affinity
		bImageNotFoundError, _ := regexp.MatchString(`image \S* not found`, err.Error())
		if bImageNotFoundError && !config.HaveNodeConstraint() {
			// Check if the image exists in the cluster
			// If exists, retry with a image affinity
			if gd.Image(config.Image) != nil {
				candidates, err = gd.Scheduler(list, config, num, true)
				retries++
			}
		}

		for ; retries < gd.createRetry && err != nil; retries++ {
			log.WithFields(log.Fields{"Name": "Swarm"}).Warnf("Failed to scheduler: %s, retrying", err)
			candidates, err = gd.Scheduler(list, config, num, true)
		}
	}

	if err != nil {
		return nil, err
	}

	if len(candidates) < num {
		return nil, errors.New("Not Enough Nodes")
	}

	preAllocs, err := gd.pendingAlloc(candidates[0:num], Type, stores, config)

	gd.scheduler.Unlock()

	return preAllocs, err
}

func (gd *Gardener) pendingAlloc(candidates []*node.Node, Type string, stores []structs.DiskStorage,
	templConfig *cluster.ContainerConfig) ([]*preAllocResource, error) {

	image, err := gd.getImageByID(templConfig.Image)
	if err != nil {
		return nil, err
	}
	parentConfig, err := database.GetUnitConfigByID(image.ImageID)
	if err != nil {
		return nil, err
	}

	allocs := make([]*preAllocResource, 0, 5)

	for i := range candidates {
		id := gd.generateUniqueID()
		engine, ok := gd.engines[candidates[i].ID]
		if !ok {
			return allocs, errors.New("Engine Not Found")
		}
		/*
			ports, err := database.SelectAvailablePorts(len(image.PortSlice))
			if err != nil {
				return preAlloc, err
			}

			for i := range ports {
				ports[i].Name = image.PortSlice[i].Name
				ports[i].UnitID = id
				ports[i].Proto = image.PortSlice[i].Proto
				ports[i].Allocated = true

			}
		*/
		unit := &unit{
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
			ports:  nil,
			parent: parentConfig,
		}

		preAlloc, err := gd.pendingAllocOneNode(engine, unit, stores, templConfig)
		allocs = append(allocs, preAlloc)
		if err != nil {
			return allocs, err
		}
	}

	return allocs, nil
}

func (gd *Gardener) pendingAllocOneNode(engine *cluster.Engine, unit *unit, stores []structs.DiskStorage, templConfig *cluster.ContainerConfig) (*preAllocResource, error) {
	preAlloc := newPreAllocResource()
	preAlloc.unit = unit

	config, err := gd.allocResource(preAlloc, engine, *templConfig, unit.Type)
	if err != nil {
		return preAlloc, fmt.Errorf("error resource alloc")
	}

	swarmID := config.SwarmID()
	if swarmID == "" {
		// Associate a Swarm ID to the container we are creating.
		swarmID = gd.generateUniqueID()
		config.SetSwarmID(swarmID)
	}

	preAlloc.swarmID = swarmID
	preAlloc.pendingContainer = &pendingContainer{
		Name:   unit.ID,
		Config: config,
		Engine: engine,
	}
	preAlloc.unit.networkings = preAlloc.networkings

	gd.pendingContainers[swarmID] = preAlloc.pendingContainer

	err = preAlloc.consistency()

	return preAlloc, nil
}

func (gd *Gardener) Scheduler(list []*node.Node, config *cluster.ContainerConfig, num int, withImageAffinity bool) ([]*node.Node, error) {
	if network := gd.Networks().Get(string(config.HostConfig.NetworkMode)); network != nil && network.Scope == "local" {
		if !config.HaveNodeConstraint() {
			config.AddConstraint("node==~" + network.Engine.Name)
		}
		config.HostConfig.NetworkMode = container.NetworkMode(network.Name)
	}

	if withImageAffinity {
		config.AddAffinity("image==" + config.Image)
	}

	nodes, err := gd.scheduler.SelectNodesForContainer(list, config)

	if withImageAffinity {
		config.RemoveAffinity("image==" + config.Image)
	}

	if err != nil {
		return nil, err
	}

	return gd.SelectNodeByCluster(nodes, num, true)
}

// listCandidateNodes returns all validated engines in the cluster, excluding pendingEngines.
func (gd *Gardener) listCandidateNodes(names []string, dcTag string, filters ...string) []*node.Node {
	gd.RLock()
	defer gd.RUnlock()

	out := make([]*node.Node, 0, len(gd.engines))

	if len(names) == 0 {

		for _, dc := range gd.datacenters {

			if dc.Type != dcTag || isStringExist(dc.ID, filters) {

				continue
			}

			list := dc.listNodeID()

			for _, id := range list {

				e, ok := gd.engines[id]
				if !ok {
					continue
				}

				if isStringExist(id, filters) {
					continue
				}

				node := node.NewNode(e)

				for _, c := range gd.pendingContainers {

					if c.Engine.ID == e.ID &&
						node.Container(c.Config.SwarmID()) == nil {

						err := node.AddContainer(c.ToContainer())
						if err != nil {
							log.Error(e.ID, err.Error())
							continue
						}
					}
				}

				out = append(out, node)
			}
		}

	} else {

		for _, name := range names {

			e, ok := gd.engines[name]
			if !ok {
				continue
			}

			if isStringExist(name, filters) {
				continue
			}

			node := node.NewNode(e)

			for _, c := range gd.pendingContainers {
				if c.Engine.ID == e.ID &&
					node.Container(c.Config.SwarmID()) == nil {

					err := node.AddContainer(c.ToContainer())
					if err != nil {
						log.Error(e.ID, err.Error())
						continue
					}
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

func (gd *Gardener) SelectNodeByCluster(nodes []*node.Node, num int, highAvailable bool) ([]*node.Node, error) {
	if len(nodes) < num {
		return nil, errors.New("Not Enough Nodes")
	}

	if !highAvailable {
		return nodes[0:num:num], nil
	}

	all, err := database.GetAllNodes()
	if err != nil {
		all = nil
	}

	m := make(map[string][]*node.Node)

	for i := range nodes {
		var dc *Datacenter = nil

		if len(all) == 0 {
			dc, err = gd.DatacenterByNode(nodes[i].ID)
		} else {
			for index := range all {
				if nodes[i].ID == all[index].ID {
					dc, err = gd.Datacenter(all[index].ClusterID)
					break
				}
			}
		}
		if err != nil || dc == nil {
			continue
		}

		if s, ok := m[dc.ID]; ok {
			m[dc.ID] = append(s, nodes[i])
		} else {
			m[dc.ID] = []*node.Node{nodes[i]}
		}

	}

	if highAvailable && len(m) < 2 {
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
			var dc *Datacenter = nil

			if len(all) == 0 {
				dc, err = gd.DatacenterByNode(nodes[i].ID)
			} else {
				for index := range all {
					if nodes[i].ID == all[index].ID {
						dc, err = gd.Datacenter(all[index].ClusterID)
						break
					}
				}
			}
			if err != nil || dc == nil {
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
