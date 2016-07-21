package swarm

import (
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
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

func (gd *Gardener) serviceScheduler() {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Recover From Panic:%v", r)
		}

		debug.PrintStack()
		logrus.Fatal("Service Scheduler Exit")
	}()

	for {
		svc := <-gd.serviceSchedulerCh

		entry := logrus.WithFields(logrus.Fields{
			"Name":   svc.Name,
			"Action": "Schedule&Alloc",
		})

		logrus.Debugf("[MG] start service Scheduler:%s", svc.Name)
		if !atomic.CompareAndSwapInt64(&svc.Status, _StatusServcieBuilding, _StatusServiceAlloction) {
			entry.Error("Status Conflict")
			continue
		}

		svc.Lock()

		resourceAlloc := make([]*pendingAllocResource, 0, len(svc.base.Modules))

		for _, module := range svc.base.Modules {

			candidates, config, err := gd.schedulerPerModule(svc, module)
			if err != nil {
				entry.WithField("Module", module.Name).Errorf("Alloction Failed %s", err)
				goto failure
			}

			pendings, err := gd.pendingAlloc(candidates, svc.ID, svc.Name, module.Type, module.Stores, config, module.Configures)
			if len(pendings) > 0 {
				resourceAlloc = append(resourceAlloc, pendings...)
			}
			if err != nil {
				entry.Errorf("gd.pendingAlloc: pendings Allocation Failed %s", err)
				goto failure
			}
			entry.Info("gd.pendingAlloc: Allocation Succeed!")
		}

		for i := range resourceAlloc {
			svc.units = append(svc.units, resourceAlloc[i].unit)
			svc.pendingContainers[resourceAlloc[i].swarmID] = resourceAlloc[i].pendingContainer
		}

		if err := createServiceResources(gd, resourceAlloc); err != nil {
			entry.Errorf("create Service Volumes Error:%s", err)
			goto failure
		}

		// scheduler success
		svc.Unlock()

		entry.Info("Alloction Success")
		logrus.Debugf("[MG]Alloction OK and put  to the ServiceToExecute: %v", resourceAlloc)

		gd.ServiceToExecute(svc)
		continue

	failure:
		logrus.Debugf("[MG]serviceScheduler Failed: %v", resourceAlloc)
		dealWithSchedulerFailure(gd, svc, resourceAlloc)

	}
}

func createServiceResources(gd *Gardener, allocs []*pendingAllocResource) (err error) {
	logrus.Debug("create Service Resources...")

	volumes := make([]*types.Volume, 0, 10)

	defer func() {
		if err != nil {
			logrus.Error("Rollback create Service volumes&IP,defer ", err)

			for _, v := range volumes {
				if v == nil {
					continue
				}
				ok, _err := gd.RemoveVolumes(v.Name)
				logrus.Debugf("Remove Volumes %s:%t,%v", v.Name, ok, _err)
			}
		}

		for _, pending := range allocs {
			if pending == nil || pending.engine == nil {
				continue
			}

			_err := removeNetworkings(pending.engine.IP, pending.networkings)
			if err != nil {
				logrus.Error(_err)
			}
		}
	}()

	for _, pending := range allocs {
		if pending == nil || pending.engine == nil {
			continue
		}
		out, err := createVolumes(pending.engine, pending.localStore, pending.sanStore)
		volumes = append(volumes, out...)

		if err != nil {
			return err
		}

		err = createNetworking(pending.engine.IP, pending.networkings)
		if err != nil {
			return err
		}
	}

	return nil
}

func dealWithSchedulerFailure(gd *Gardener, svc *Service, pendings []*pendingAllocResource) {
	err := gd.Recycle(pendings)
	if err != nil {
		logrus.Error("Recycle Failed", err)
	}

	// scheduler failed
	gd.scheduler.Lock()
	for i := range pendings {
		delete(gd.pendingContainers, pendings[i].swarmID)
	}
	gd.scheduler.Unlock()

	svc.pendingContainers = make(map[string]*pendingContainer)
	svc.units = make([]*unit, 0, 10)

	svc.Service.SetServiceStatus(_StatusServiceAlloctionFailed, time.Now())

	svc.Unlock()
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

func (gd *Gardener) schedulerPerModule(svc *Service, module structs.Module) ([]*node.Node, *cluster.ContainerConfig, error) {
	entry := logrus.WithFields(logrus.Fields{
		"svcName": svc.Name,
		"Module":  module.Type,
	})

	_, num, err := getServiceArch(module.Arch)
	if err != nil {
		entry.Errorf("Parse Module.Arch:%s,Error:%v", module.Arch, err)

		return nil, nil, err
	}

	config, err := templateConfig(gd, module)
	if err != nil {
		return nil, nil, err
	}
	// TODO:maybe remove
	_type := module.Type
	if _type == _SwitchManagerType {
		_type = _ProxyType
	}

	highAvaliable := svc.HighAvailable
	if length := len(module.Clusters); length > 1 {
		highAvaliable = true
	} else if length == 1 {
		highAvaliable = false
	}
	for i := range module.Stores {
		if !store.IsLocalStore(module.Stores[i].Type) {
			highAvaliable = true
		}
	}

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	list, err := listCandidates(module.Clusters, _type)
	if err != nil {
		return nil, nil, err
	}
	logrus.Debugf("all candidate nodes num:%d,filter by Type:'%s'", len(list), _type)

	list = gd.shortIdleStoreFilter(list, module.Stores, _type, num)

	candidates := gd.listCandidateNodes(list)

	candidates, err = gd.Scheduler(config, num, candidates, false, highAvaliable)
	if err != nil {
		return nil, nil, err
	}

	return candidates, config, nil
}

func (gd *Gardener) pendingAlloc(candidates []*node.Node, svcID, svcName, _type string, stores []structs.DiskStorage,
	templConfig *cluster.ContainerConfig, configures map[string]interface{}) ([]*pendingAllocResource, error) {
	entry := logrus.WithFields(logrus.Fields{"Name": svcName, "Module": _type})

	imageID, ok := templConfig.Labels[_ImageIDInRegistryLabelKey]
	if !ok || imageID == "" {
		return nil, fmt.Errorf("Missing Value of ContainerConfig.Labels[_ImageIDInRegistryLabelKey]")
	}

	image, parentConfig, err := database.GetImageAndUnitConfig(imageID)
	if err != nil {
		err = fmt.Errorf("Get Image And UnitConfig Error:%s", err)
		entry.Error(err)
		return nil, err
	}

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	allocs := make([]*pendingAllocResource, 0, 5)

	for i := range candidates {

		config := cloneContainerConfig(templConfig)
		id := gd.generateUniqueID()

		engine, ok := gd.engines[candidates[i].ID]
		if !ok || engine == nil {
			err := fmt.Errorf("Not Found Engine '%s':'%s'", candidates[i].ID, candidates[i].Addr)
			entry.Error(err)

			return allocs, err
		}

		unit := &unit{
			Unit: database.Unit{
				ID:            id,
				Name:          id[:8] + "_" + svcName,
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
			engine:     engine,
			ports:      nil,
			parent:     &parentConfig,
			configures: configures,
		}

		forbid, can := unit.CanModify(configures)
		if !can {
			return allocs, fmt.Errorf("Forbid modifying service config,%s", forbid)
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

	err = gd.allocStorage(pending, engine, config, stores, false)
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

	gd.pendingContainers[swarmID] = pending.pendingContainer

	err = pending.consistency()
	if err != nil {
		entry.Errorf("Pending Allocation Resouces,Consistency Error:%s", err)
	}

	return pending, err
}

func (gd *Gardener) Scheduler(config *cluster.ContainerConfig, num int,
	list []*node.Node, withImageAffinity, highAvaliable bool) ([]*node.Node, error) {

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
		logrus.Debugf("[MG] gd.scheduler.SelectNodesForContainer fail(swarm level) :", err)
		return nil, err
	}

	logrus.Debugf("[MG] gd.scheduler.SelectNodesForContainer ok(swarm level) ndoes:%v", nodes)
	return gd.SelectNodeByCluster(nodes, num, highAvaliable)
}

func listCandidates(clusters []string, _type string) ([]database.Node, error) {
	list, err := database.ListNodesByClusters(clusters, _type, true)
	if err != nil {
		logrus.Errorf("Search in Database Error: %s", err)

		return nil, err
	}

	out := make([]database.Node, 0, len(list))
	for i := range list {
		if list[i].Status != _StatusNodeEnable {
			continue
		}
		out = append(out, list[i])
	}

	return out, nil
}

// listCandidateNodes returns all validated engines in the cluster, excluding pendingEngines.
func (gd *Gardener) listCandidateNodes(list []database.Node) []*node.Node {
	gd.RLock()
	defer gd.RUnlock()

	out := make([]*node.Node, 0, len(list))

	for i := range list {
		node := gd.checkNode(list[i].EngineID)
		if node != nil {
			out = append(out, node)
		}
	}

	logrus.Debugf("Candidate Nodes:%d", len(out))

	return out
}

func (gd *Gardener) checkNode(id string) *node.Node {
	e, ok := gd.engines[id]
	if !ok {
		logrus.Debugf("Not Found Engine %s", id)

		return nil
	}

	node := node.NewNode(e)

	for _, pc := range gd.pendingContainers {

		if pc.Engine.ID == e.ID && node.Container(pc.Config.SwarmID()) == nil {

			err := node.AddContainer(pc.ToContainer())
			if err != nil {
				logrus.Error(e.ID, err)

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
		return nil, errors.New("Not Enough Nodes For Match")
	}

	if !highAvailable || num == 1 {
		return nodes[0:num:num], nil
	}

	all, err := database.GetAllNodes()
	if err != nil {
		logrus.Warnf("[**MG**]SelectNodeByCluster::database.GetAllNodes fail", err)
		all = nil
	}

	dcMap := make(map[string][]*node.Node)

	for i := range nodes {
		dcID := ""

		if len(all) == 0 {
			node, err := database.GetNode(nodes[i].ID)
			if err != nil {
				logrus.Warnf("[MG]SelectNodeByCluster::DatacenterByNode fail", err)
				continue
			}
			dcID = node.ClusterID

			logrus.Debugf("[**MG]len(all) == 0 the dc :%s", dcID)
		} else {
			for index := range all {
				if nodes[i].ID == all[index].EngineID {
					dcID = all[index].ClusterID
					break
				}
			}
			logrus.Debugf("[MG]len(all) = %d, the DC:%s", len(all), dcID)
		}
		if err != nil || dcID == "" {
			logrus.Warningf("%d Node %s fail,%v", i, nodes[i].ID, err)
			continue
		}

		if list, ok := dcMap[dcID]; ok {
			dcMap[dcID] = append(list, nodes[i])
		} else {
			list := make([]*node.Node, 1, len(nodes)/2)
			list[0] = nodes[i]
			dcMap[dcID] = list
		}

		logrus.Debugf("DC %s Append Node:%s,len=%d", dcID, nodes[i].Name, len(dcMap[dcID]))
	}

	logrus.Debugf("[MG]highAvailable:%t, num :%d ,dcMap len=%d", highAvailable, num, len(dcMap))

	if highAvailable && num > 1 && len(dcMap) < 2 {
		return nil, errors.New("Not Enough Cluster For Match")
	}

	candidates := make([]*node.Node, num)

	for index := 0; index < num && len(dcMap) > 0; {

		for key, list := range dcMap {
			if len(list) == 0 {
				delete(dcMap, key)
				continue
			}

			if list[0] != nil {
				candidates[index] = list[0]
				index++

				if index == num {
					dcMap = nil

					return candidates, nil
				}
			}

			if len(list[1:]) > 0 {
				dcMap[key] = list[1:]
			} else {
				delete(dcMap, key)
			}

		}
	}

	return nil, errors.New("Not Enough Cluster&Node For Match")
}
