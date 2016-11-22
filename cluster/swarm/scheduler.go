package swarm

import (
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
)

func (gd *Gardener) serviceScheduler(svc *Service, task *database.Task) (err error) {
	entry := logrus.WithFields(logrus.Fields{
		"Name":   svc.Name,
		"Action": "Schedule",
	})
	resourceAlloc := make([]*pendingAllocResource, 0, len(svc.base.Modules))

	defer func() {
		if err == nil {
			_err := svc.statusLock.SetStatus(statusServiceAllocated)
			if _err != nil {
				entry.Errorf("Set Service Status:statusServiceAllocated(%x),%+v", statusServiceAllocated, _err)
			}
			return
		}

		entry.WithError(err).Errorf("scheduler failed")

		if err != nil && len(resourceAlloc) > 0 {

			// scheduler failed
			gd.scheduler.Lock()
			for i := range resourceAlloc {
				delete(gd.pendingContainers, resourceAlloc[i].swarmID)
			}
			gd.scheduler.Unlock()
		}

		_err := svc.statusLock.SetStatus(statusServiceAllocateFailed)
		if _err != nil {
			entry.Errorf("Set Service Status:statusServiceAllocateFailed(%x),%+v", statusServiceAllocateFailed, _err)
		}
	}()

	entry.Debug("start service Scheduler")

	for _, module := range svc.base.Modules {

		candidates, config, err := gd.schedulerPerModule(svc, module)
		if err != nil {
			entry.WithField("Module", module.Name).WithError(err).Error("scheduler failed")

			return err
		}

		pendings, err := gd.pendingAlloc(candidates, svc.ID, svc.Name, module.Type, module.Stores, config, module.Configures)
		if len(pendings) > 0 {
			resourceAlloc = append(resourceAlloc, pendings...)
		}
		if err != nil {
			entry.WithError(err).Error("pendings alloc failed")

			return err
		}
		entry.Info("gd.pendingAlloc: allocation Succeed!")
	}

	for i := range resourceAlloc {
		svc.units = append(svc.units, resourceAlloc[i].unit)
	}

	if err := createServiceResources(gd, resourceAlloc); err != nil {
		entry.WithError(err).Error("create Service volumes")

		return err
	}

	// scheduler success
	entry.Info("Alloction Success")

	return nil
}

func createServiceResources(gd *Gardener, allocs []*pendingAllocResource) (err error) {
	logrus.Debug("create Service resources...")

	volumes := make([]*types.Volume, 0, 10)

	for _, pending := range allocs {
		if pending == nil || pending.engine == nil {
			continue
		}

		out, err := createVolumes(pending.engine, pending.localStore)
		volumes = append(volumes, out...)
		if err != nil {
			return err
		}
	}

	return nil
}

func templateConfig(gd *Gardener, module structs.Module) (*cluster.ContainerConfig, error) {
	hostConfig := container.HostConfig{
		Resources: container.Resources{
			Memory:     module.HostConfig.Memory,
			CpusetCpus: module.HostConfig.CpusetCpus,
		},
	}

	config := cluster.BuildContainerConfig(module.Config, hostConfig, module.NetworkingConfig)
	config = buildContainerConfig(config)

	if err := validContainerConfig(config); err != nil {

		return nil, err
	}

	image, imageIDLabel, err := gd.getImageName(module.Config.Image, module.Name, module.Version)
	if err != nil {
		return nil, err
	}
	config.Config.Image = image
	config.Config.Labels[_ImageIDInRegistryLabelKey] = imageIDLabel

	return config, nil
}

func (gd *Gardener) schedulerPerModule(svc *Service, module structs.Module) ([]*node.Node, *cluster.ContainerConfig, error) {
	entry := logrus.WithFields(logrus.Fields{
		"Service": svc.Name,
		"Module":  module.Type,
	})

	_, num, err := parseServiceArch(module.Arch)
	if err != nil {
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

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	list, err := listCandidates(module.Clusters, _type)
	if err != nil {
		return nil, nil, err
	}

	entry.Debugf("all candidate nodes num:%d,filter by Type:'%s'", len(list), _type)

	list, err = gd.resourceFilter(list, module, num)
	if err != nil {
		return nil, nil, err
	}

	candidates := gd.listCandidateNodes(list)

	candidates, err = gd.dispatch(config, num, candidates, false, module.HighAvailable)
	if err != nil {
		return nil, nil, err
	}

	return candidates, config, nil
}

func (gd *Gardener) pendingAlloc(candidates []*node.Node,
	svcID, svcName, _type string,
	stores []structs.DiskStorage,
	templConfig *cluster.ContainerConfig,
	configures map[string]interface{}) ([]*pendingAllocResource, error) {

	entry := logrus.WithFields(logrus.Fields{
		"Name":   svcName,
		"Module": _type,
	})

	imageID, ok := templConfig.Labels[_ImageIDInRegistryLabelKey]
	if !ok || imageID == "" {
		return nil, errors.New("ContainerConfig.Labels[_ImageIDInRegistryLabelKey] is required")
	}

	image, parentConfig, err := database.GetImageAndUnitConfig(imageID)
	if err != nil {
		entry.Error(err)
		return nil, err
	}

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	allocs := make([]*pendingAllocResource, 0, 5)

	for i := range candidates {

		config := cloneContainerConfig(templConfig)

		engine, ok := gd.engines[candidates[i].ID]
		if !ok || engine == nil {
			return allocs, errors.Errorf("not found Engine '%s':'%s'", candidates[i].ID, candidates[i].Addr)
		}

		networkMode := "host"
		if name := config.HostConfig.NetworkMode.NetworkName(); name != "" {
			networkMode = name
		}

		id := gd.generateUniqueID()
		unit := &unit{
			Unit: database.Unit{
				ID:            id,
				Name:          id[:8] + "_" + svcName,
				Type:          _type,
				ServiceID:     svcID,
				ImageID:       image.ID,
				ImageName:     image.Name + ":" + image.Version,
				EngineID:      engine.ID,
				Status:        statusUnitAllocting,
				LatestError:   "",
				CheckInterval: 0,
				NetworkMode:   networkMode,
				CreatedAt:     time.Now(),
			},
			engine:     engine,
			ports:      nil,
			parent:     &parentConfig,
			configures: configures,
		}

		forbid, can := unit.CanModify(configures)
		if !can {
			return allocs, errors.Errorf("forbid modifying service config,%s", forbid)
		}

		if err := unit.factory(); err != nil {
			entry.Error(err)

			return allocs, err
		}

		if err := database.InsertUnit(unit.Unit); err != nil {
			entry.Error(err)

			return allocs, err
		}

		preAlloc, err := gd.pendingAllocOneNode(engine, unit, stores, config)
		allocs = append(allocs, preAlloc)
		if err != nil {
			atomic.StoreInt64(&unit.Status, statusUnitAlloctionFailed)
			unit.LatestError = err.Error()

			_err := unit.saveToDisk()
			if err != nil {
				entry.WithError(_err).Error("update Unit")
			}

			entry.Errorf("pendingAlloc:alloc resource %+v", err)

			return allocs, err
		}
	}

	entry.Info("pendingAlloc: allocation succeed!")

	return allocs, nil
}

func (gd *Gardener) pendingAllocOneNode(engine *cluster.Engine, unit *unit,
	stores []structs.DiskStorage, config *cluster.ContainerConfig) (*pendingAllocResource, error) {

	entry := logrus.WithFields(logrus.Fields{
		"Engine": engine.Name,
		"Unit":   unit.Name,
	})

	pending := newPendingAllocResource()
	pending.unit = unit
	pending.engine = engine

	// constraint := fmt.Sprintf("constraint:node==%s", engine.ID)
	// config.Env = append(config.Env, constraint)
	// conflicting options:hostname and the network mode
	// config.Hostname = engine.ID
	// config.Domainname = engine.Name

	// Alloc CPU
	cpuset, err := gd.allocCPUs(engine, config.HostConfig.CpusetCpus)
	if err != nil {
		entry.WithError(err).Errorf("alloc CPU '%s'", config.HostConfig.CpusetCpus)
		return pending, err
	}
	config.HostConfig.CpusetCpus = cpuset

	err = gd.allocNetworking(pending, config)
	if err != nil {
		entry.WithError(err).Error("alloc networking")

		return pending, err
	}

	err = gd.allocStorage(pending, engine, config, stores, false)
	if err != nil {
		entry.WithError(err).Error("alloc Storage")

		return pending, err
	}

	config.Env = append(config.Env, fmt.Sprintf("C_NAME=%s", unit.Name))
	config.Labels["unit_id"] = unit.ID
	swarmID := config.SwarmID()
	if swarmID == "" {
		// Associate a Swarm ID to the container we are creating.
		swarmID = unit.ID
		config.SetSwarmID(swarmID)
	} else {
		logrus.Warn("ContainerConfig.SwarmID() should be null but got %s", swarmID)
	}

	pending.unit.config = config
	pending.swarmID = swarmID

	gd.pendingContainers[swarmID] = &pendingContainer{
		Name:   unit.Name,
		Config: config,
		Engine: engine,
	}

	atomic.StoreInt64(&pending.unit.Status, statusUnitAllocted)

	return pending, err
}

func (gd *Gardener) dispatch(config *cluster.ContainerConfig, num int,
	list []*node.Node, withImageAffinity, highAvaliable bool) ([]*node.Node, error) {

	if len(list) < num {
		err := errors.Errorf("not enough candidate Nodes for allocation,%d<%d", len(list), num)
		logrus.Warn(err)

		return nil, err
	}

	candidates, err := gd.runScheduler(list, config, num, withImageAffinity, highAvaliable)
	if err != nil {
		logrus.WithError(err).Warn("runScheduler failed")

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
		logrus.WithError(err).Warn("runScheduler failed")

		return nil, err
	}

	if len(candidates) < num {
		err := errors.Errorf("not enough match condition Nodes after retries,%d<%d", len(candidates), num)
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
		logrus.WithError(err).Errorf("gd.scheduler.SelectNodesForContainer failed(swarm scheduler level)")

		return nil, errors.Wrap(err, "scheduler.SelectNodesForContainer")
	}

	logrus.Debugf("gd.scheduler.SelectNodesForContainer ok(swarm scheduler level) ndoes:%d", len(nodes))

	return gd.selectNodeByCluster(nodes, num, highAvaliable)
}

func listCandidates(clusters []string, _type string) ([]database.Node, error) {
	list, err := database.ListNodesByClusters(clusters, _type, true)
	if err != nil {
		return nil, err
	}

	out := make([]database.Node, 0, len(list))
	for i := range list {
		if list[i].Status != statusNodeEnable {
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

	logrus.Debugf("candidate Nodes:%d", len(out))

	return out
}

func (gd *Gardener) checkNode(id string) *node.Node {
	e, ok := gd.engines[id]
	if !ok {
		logrus.Debugf("not found Engine %s", id)

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

func (gd *Gardener) selectNodeByCluster(nodes []*node.Node, num int, highAvailable bool) ([]*node.Node, error) {
	if len(nodes) < num {
		return nil, errors.New("not enough Nodes for select")
	}

	fmt.Println("selectNodeByCluster....  highAvailable =", highAvailable, num)
	for i, n := range nodes {
		fmt.Println(i, n.Name, n.Addr, n.UsedCpus)
	}

	if !highAvailable || num == 1 {
		return nodes[0:num:num], nil
	}

	all, err := database.GetAllNodes()
	if err != nil {
		logrus.Warnf("database.GetAllNodes failed,%+v", err)

		all = nil
	}

	dcMap := make(map[string][]*node.Node)

	for i := range nodes {
		dcID := ""

		if len(all) == 0 {

			node, err := database.GetNode(nodes[i].ID)
			if err != nil {
				logrus.Warnf("SelectNodeByCluster::DatacenterByNode failed,%+v", err)

				continue
			}
			dcID = node.ClusterID

			logrus.Debugf("len(all) = %d, DC:%s", len(all), dcID)

		} else {

			for index := range all {

				if nodes[i].ID == all[index].EngineID {
					dcID = all[index].ClusterID

					break
				}
			}

			logrus.Debugf("len(all) = %d, DC:%s", len(all), dcID)
		}
		if err != nil || dcID == "" {
			logrus.Warningf("%d Node %s failed,%v", i, nodes[i].ID, err)
			continue
		}

		if list, ok := dcMap[dcID]; ok {
			dcMap[dcID] = append(list, nodes[i])
		} else {
			list := make([]*node.Node, 1, len(nodes)/2)
			list[0] = nodes[i]
			dcMap[dcID] = list
		}

		logrus.Debugf("DC %s append Node:%s,len=%d", dcID, nodes[i].Name, len(dcMap[dcID]))
	}

	logrus.Debugf("highAvailable=%t, num=%d,len(dcMap)=%d", highAvailable, num, len(dcMap))

	if highAvailable && num > 1 && len(dcMap) < 2 {
		return nil, errors.New("not enough Cluster for Match")
	}

	candidates := make([]*node.Node, num)

	for index := 0; index < num && len(dcMap) > 0; {

		for key, list := range dcMap {
			fmt.Println("cluster:", key, len(list))
			if len(list) == 0 {
				delete(dcMap, key)
				continue
			}

			if list[0] != nil {
				candidates[index] = list[0]
				index++

				if index == num {
					dcMap = nil

					fmt.Println("Output:")
					for i, n := range candidates {
						fmt.Println(i, n.Name, n.Addr, n.UsedCpus)
					}

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

	return nil, errors.New("not enough Cluster&Node for Match")
}
