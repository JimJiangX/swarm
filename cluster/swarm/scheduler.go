package swarm

import (
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
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

		debug.PrintStack()
		logrus.Fatal("Service Scheduler Exit,%s", err)
	}()

	for {
		svc := <-gd.serviceSchedulerCh

		entry := logrus.WithFields(logrus.Fields{
			"Name":   svc.Name,
			"Action": "Alloc",
		})

		logrus.Debugf("[mg] start serviceScheduler:%s", svc.Name)
		if !atomic.CompareAndSwapInt64(&svc.Status, _StatusServcieBuilding, _StatusServiceAlloction) {
			entry.Error("Status Conflict")
			continue
		}

		svc.Lock()

		resourceAlloc := make([]*pendingAllocResource, 0, len(svc.base.Modules))

		for i := range svc.base.Modules {
			preAlloc, err := gd.BuildPendingContainersPerModule(svc, svc.base.Modules[i])
			if len(preAlloc) > 0 {
				resourceAlloc = append(resourceAlloc, preAlloc...)
			}
			if err != nil {
				entry.WithField("Module", svc.base.Modules[i].Name).Errorf("Alloction Failed %s", err)
				goto failure
			}
		}

		for i := range resourceAlloc {
			svc.units = append(svc.units, resourceAlloc[i].unit)
			svc.pendingContainers[resourceAlloc[i].swarmID] = resourceAlloc[i].pendingContainer
		}

		// scheduler success
		svc.Unlock()

		entry.Info("Alloction Success")
		logrus.Debugf("[MG]Alloction OK and put  to the ServiceToExecute: %v", resourceAlloc)

		gd.ServiceToExecute(svc)
		continue

	failure:
		logrus.Debugf("[MG]serviceScheduler Failed: %v", resourceAlloc)
		err = gd.Recycle(resourceAlloc)
		if err != nil {
			entry.Error("Recycle Failed", err)
		}

		// scheduler failed
		gd.scheduler.Lock()
		for i := range resourceAlloc {
			delete(gd.pendingContainers, resourceAlloc[i].swarmID)
		}
		gd.scheduler.Unlock()

		svc.pendingContainers = make(map[string]*pendingContainer)
		svc.units = make([]*unit, 0, 10)

		svc.Service.SetServiceStatus(_StatusServiceAlloctionFailed, time.Now())

		svc.Unlock()
	}

	return err
}

func templateConfig(gd *Gardener, module structs.Module) (*cluster.ContainerConfig, error) {

	config := cluster.BuildContainerConfig(module.Config, module.HostConfig, module.NetworkingConfig)
	config = buildContainerConfig(config)

	if err := validateContainerConfig(config); err != nil {
		logrus.Warnf("Container Config Validate:%s", err)

		return nil, err
	}
	logrus.Infof("Build Container Config,Validate OK:%+v", config)

	image, imageID_Label, err := gd.GetImageName(module.Config.Image, module.Name, module.Version)
	if err != nil {
		return nil, err
	}
	config.Config.Image = image
	config.Config.Labels[_ImageIDInRegistryLabelKey] = imageID_Label

	return config, nil
}

func (gd *Gardener) BuildPendingContainersPerModule(svc *Service, module structs.Module) ([]*pendingAllocResource, error) {
	entry := logrus.WithFields(logrus.Fields{
		"svcName": svc.Name,
		"Module":  module.Type,
	})

	_, num, err := getServiceArch(module.Arch)
	if err != nil {
		entry.Errorf("Parse Module.Arch:%s,Error:%v", module.Arch, err)

		return nil, err
	}

	// TODO:maybe remove
	_type := module.Type
	if _type == _SwitchManagerType {
		_type = _ProxyType
	}

	filters := gd.listShortIdleStore(module.Stores, _type, num)
	logrus.Debugf("[MG] %s,%s,%s:first filters of storage:%s", module.Stores, module.Type, num, filters)

	config, err := templateConfig(gd, module)
	if err != nil {
		return nil, err
	}

	highAvaliable := svc.HighAvailable
	for i := range module.Stores {
		if !store.IsLocalStore(module.Stores[i].Type) {
			highAvaliable = true
		}
	}

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	candidates, err := gd.Scheduler(config, _type, num, module.Candidates, filters, false, highAvaliable)
	if err != nil {
		return nil, err
	}

	return gd.BuildPendingContainers(svc, module.Type, candidates, module.Stores, config)
}

func (gd *Gardener) BuildPendingContainers(svc *Service, _type string, candidates []*node.Node,
	stores []structs.DiskStorage, config *cluster.ContainerConfig) ([]*pendingAllocResource, error) {

	entry := logrus.WithFields(logrus.Fields{"Name": svc.Name, "Module": _type})

	pendings, err := gd.pendingAlloc(candidates, svc.ID, svc.Name, _type, stores, config)
	if err != nil {
		entry.Errorf("gd.pendingAlloc: pendings Allocation Failed %s", err)
		return pendings, err
	}

	entry.Info("gd.pendingAlloc: Allocation Succeed!")

	return pendings, nil
}

func (gd *Gardener) pendingAlloc(candidates []*node.Node, svcID, svcName, _type string,
	stores []structs.DiskStorage, templConfig *cluster.ContainerConfig) ([]*pendingAllocResource, error) {

	entry := logrus.WithFields(logrus.Fields{"Name": svcName, "Module": _type})

	imageID, ok := templConfig.Labels[_ImageIDInRegistryLabelKey]
	if !ok || imageID == "" {
		return nil, fmt.Errorf("Missing Value of ContainerConfig.Labels[_ImageIDInRegistryLabelKey]")
	}

	image, err := database.QueryImageByID(imageID)
	if err != nil {
		return nil, err
	}

	parentConfig, err := database.GetUnitConfigByID(image.ID)
	if err != nil {
		entry.Errorf("Not Found Template Config File,Error:%s", err)

		return nil, err
	}

	allocs := make([]*pendingAllocResource, 0, 5)

	for i := range candidates {
		config := cloneContainerConfig(templConfig)
		id := gd.generateUniqueID()

		engine, ok := gd.engines[candidates[i].ID]
		if !ok || engine == nil {
			err := fmt.Errorf("Engine %s Not Found", candidates[i].ID)
			entry.Error(err)

			return allocs, err
		}

		unit := &unit{
			Unit: database.Unit{
				ID:            id,
				Name:          string(id[:8]) + "_" + svcName,
				Type:          _type,
				ServiceID:     svcID,
				ImageID:       image.ID,
				ImageName:     image.Name + "_" + image.Version,
				EngineID:      engine.ID,
				ConfigID:      parentConfig.ID,
				Status:        0,
				CheckInterval: 0,
				NetworkMode:   config.HostConfig.NetworkMode.NetworkName(),
				CreatedAt:     time.Now(),
			},
			engine: engine,
			ports:  nil,
			parent: parentConfig,
		}

		if err := unit.factory(); err != nil {
			entry.Error(err)

			return allocs, err
		}

		preAlloc, err := gd.pendingAllocOneNode(engine, unit, stores, config)
		allocs = append(allocs, preAlloc)
		if err != nil {
			entry.Errorf("pendingAlloc:Alloc Resource %s", err)

			return allocs, err
		}
	}

	entry.Info("pendingAlloc: Allocation Succeed!")

	return allocs, nil
}

func (gd *Gardener) pendingAllocOneNode(engine *cluster.Engine, unit *unit,
	stores []structs.DiskStorage, config *cluster.ContainerConfig) (*pendingAllocResource, error) {

	entry := logrus.WithFields(logrus.Fields{
		"Engine": engine.Name,
		"Unit":   unit.Name,
	})

	pending, err := gd.allocResource(unit, engine, config)
	if err != nil {
		err = fmt.Errorf("Alloc Resource Error:%s", err)
		entry.Error(err)

		return pending, err
	}

	err = gd.allocStorage(pending, engine, config, stores)
	if err != nil {
		entry.Errorf("Alloc Storage Error:%s", err)

		return pending, err
	}

	config.Env = append(config.Env, fmt.Sprintf("C_NAME=%s", unit.Name))
	config.Labels["unit_id"] = unit.ID
	swarmID := config.SwarmID()
	if swarmID == "" {
		// Associate a Swarm ID to the container we are creating.
		swarmID = gd.generateUniqueID()
		config.SetSwarmID(swarmID)
	} else {
		logrus.Errorf("ContainerConfig.SwarmID() Should be null but got %s", swarmID)
	}

	pending.unit.config = config
	pending.swarmID = swarmID
	pending.pendingContainer = &pendingContainer{
		Name:   unit.Name,
		Config: config,
		Engine: engine,
	}
	pending.unit.networkings = pending.networkings

	gd.pendingContainers[swarmID] = pending.pendingContainer

	err = pending.consistency()
	if err != nil {
		entry.Errorf("Pending Allocation Resouces,Consistency Error:%s", err)
	}

	return pending, err
}

func (gd *Gardener) Scheduler(config *cluster.ContainerConfig,
	_type string, num int, nodes, filters []string,
	withImageAffinity, highAvaliable bool) ([]*node.Node, error) {

	list := gd.listCandidateNodes(nodes, _type, filters...)
	logrus.Debugf("filters num:%d,candidate nodes num:%d", len(filters), len(list))

	if len(list) < num {
		err := fmt.Errorf("Not Enough Candidate Nodes For Allocation,%d<%d", len(list), num)
		logrus.Warn(err)

		return nil, err
	}

	candidates, err := gd.runScheduler(list, config, num, withImageAffinity, highAvaliable)
	if err != nil {
		logrus.Warnf("Failed to scheduler: %s", err)

		var retries int64
		//  fails with image not found, then try to reschedule with image affinity
		bImageNotFoundError, _ := regexp.MatchString(`image \S* not found`, err.Error())
		if bImageNotFoundError && !config.HaveNodeConstraint() {
			// Check if the image exists in the cluster
			// If exists, retry with a image affinity
			if gd.Image(config.Image) != nil {
				candidates, err = gd.runScheduler(list, config, num, true, highAvaliable)
				retries++
			}
		}

		for ; retries < gd.createRetry && err != nil; retries++ {
			logrus.Warnf("Failed to scheduler: %s, retrying", err)
			candidates, err = gd.runScheduler(list, config, num, true, highAvaliable)
		}
	}
	if err != nil {
		logrus.Warnf("Failed to scheduler: %s", err)

		return nil, err
	}

	if len(candidates) < num {
		err := fmt.Errorf("Not Enough Match Condition Nodes After Retries,%d<%d", len(candidates), num)
		logrus.Error(err)

		return nil, err
	}

	return candidates, nil
}

func (gd *Gardener) runScheduler(list []*node.Node, config *cluster.ContainerConfig, num int, withImageAffinity, highAvaliable bool) ([]*node.Node, error) {
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
		logrus.Debugf("[**MG**] gd.scheduler.SelectNodesForContainer fail(swarm level) :", err)
		return nil, err
	}

	logrus.Debugf("[**MG**] gd.scheduler.SelectNodesForContainer ok(swarm level) ndoes:%v", nodes)
	return gd.SelectNodeByCluster(nodes, num, highAvaliable)
}

// listCandidateNodes returns all validated engines in the cluster, excluding pendingEngines.
func (gd *Gardener) listCandidateNodes(candidates []string, _type string, filters ...string) []*node.Node {
	gd.RLock()
	defer gd.RUnlock()

	out := make([]*node.Node, 0, len(gd.engines))

	if len(candidates) == 0 {

		list, err := database.ListNodeByClusterType(_type, true)
		if err != nil {
			logrus.Errorf("Search in Database Error: %s", err)

			return nil
		}

		for i := range list {

			if list[i].Status != _StatusNodeEnable ||
				isStringExist(list[i].ClusterID, filters) {

				continue
			}

			node := gd.checkNode(list[i].EngineID, filters)
			if node != nil {
				out = append(out, node)
			}
		}

	} else {

		logrus.Debugf("Candidates From Assigned %s", candidates)

		for i := range candidates {

			node := gd.checkNode(candidates[i], filters)
			if node != nil {
				out = append(out, node)
			}
		}
	}

	logrus.Debugf("Candidate Nodes:%d", len(out))

	return out
}

func (gd *Gardener) checkNode(id string, filters []string) *node.Node {
	e, ok := gd.engines[id]
	if !ok {
		logrus.Debugf("Not Found Engine %s", id)

		return nil
	}

	if isStringExist(id, filters) {
		logrus.Debug(id, "IN", filters)
		return nil
	}

	node := node.NewNode(e)

	for _, pc := range gd.pendingContainers {

		if pc.Engine.ID == e.ID && node.Container(pc.Config.SwarmID()) == nil {

			err := node.AddContainer(pc.ToContainer())
			if err != nil {
				logrus.Error(e.ID, err.Error())

				return nil
			}
		}
	}

	return node
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
		logrus.Warnf("[**MG**]SelectNodeByCluster::database.GetAllNodes fail", err)
		all = nil
	}

	m := make(map[string][]*node.Node)

	for i := range nodes {
		var dc *Datacenter = nil

		if len(all) == 0 {
			dc, err = gd.DatacenterByNode(nodes[i].ID)
			if err != nil {
				logrus.Warnf("[**MG**]SelectNodeByCluster::DatacenterByNode fail", err)
			}
			logrus.Debugf("[**MG**]len(all) == 0 the dc :%v", dc)
		} else {
			for index := range all {
				logrus.Debugf("the nodes[i].ID == all[index].ID :%s:%s  ", nodes[i].ID, all[index].EngineID)
				if nodes[i].ID == all[index].EngineID {
					dc, err = gd.Datacenter(all[index].ClusterID)
					if err != nil {
						logrus.Warnf("[**MG**] SelectNodeByCluster::database.Datacenter fail", err)
					}
					break
				}
			}
			logrus.Debugf("[**MG**]len(all) != 0 the dc :%v", dc)
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

	logrus.Warnf("[**MG**]highAvailable:%v, num :%d ,m length:%d", highAvailable, num, len(m))

	//	if highAvailable && len(m) < 2 {
	//		return nil, errors.New("Not Match")
	//	}

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

	logrus.Debugf("[**MG**]SelectNodeByCluster end with Not Match ")
	return nil, errors.New("Not Match")
}
