package swarm

import (
	"fmt"
	"strconv"

	"github.com/Sirupsen/logrus"
	ctypes "github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/utils"
)

func (gd *Gardener) UnitRebuild(name string, candidates []string, hostConfig *ctypes.HostConfig) error {
	table, err := database.GetUnit(name)
	if err != nil {
		return fmt.Errorf("Not Found Unit %s,error:%s", name, err)
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return err
	}

	svc.RLock()
	index, module := 0, structs.Module{}
	filters := make([]string, len(svc.units))
	for i, u := range svc.units {
		filters[i] = u.EngineID
		if u.Name == name {
			index = i
		}
	}
	u := svc.units[index]

	for i := range svc.base.Modules {
		if u.Type == svc.base.Modules[i].Type {
			module = svc.base.Modules[i]
			break
		}
	}
	svc.RUnlock()
	if err != nil {
		return err
	}

	err = u.stopContainer(0)
	if err != nil {
		return err
	}
	err = removeNetworking(u.engine.IP, u.networkings)
	if err != nil {
		return err
	}

	config, err := resetContainerConfig(u.container.Config, hostConfig)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		table, err := database.GetNode(u.EngineID)
		if err != nil {
			return err
		}
		nodes, err := database.ListNodeByCluster(table.ClusterID)
		if err != nil {
			return err
		}
		candidates = make([]string, len(nodes))
		for i := range nodes {
			candidates[i] = nodes[i].EngineID
		}
	} else {
		candidates, err = database.ListEnginesByNodes(candidates)
		if err != nil {
			return err
		}
	}

	engine, err := gd.selectEngine(config, module, candidates, filters)
	if err != nil {
		return err
	}
	u.engine = engine
	u.EngineID = engine.ID

	ncpu, err := parseCpuset(config.HostConfig.CpusetCpus)
	if err != nil {
		return err
	}
	cpuset, err := gd.allocCPUs(engine, ncpu)
	if err != nil {
		logrus.Errorf("Alloc CPU %d Error:%s", ncpu, err)
		return err
	}
	config.HostConfig.CpusetCpus = cpuset

	_, node, err := gd.GetNode(engine.ID)
	if err != nil {
		err := fmt.Errorf("Not Found Node %s,Error:%s", engine.Name, err)
		logrus.Error(err)

		return err
	}

	pending := pendingAllocStore{
		localStore: make([]localVolume, 0, len(module.Stores)),
		sanStore:   make([]string, 0, 3),
	}
	for i := range module.Stores {
		name := fmt.Sprintf("%s_%s_LV", u.Unit.Name, module.Stores[i].Name)

		if !store.IsLocalStore(module.Stores[i].Type) {
			continue
		}
		lv, err := node.localStorageAlloc(name, u.Unit.ID, module.Stores[i].Type, module.Stores[i].Size)
		if err != nil {
			return err
		}
		pending.localStore = append(pending.localStore, localVolume{
			lv:   lv,
			size: module.Stores[i].Size,
		})
	}

	swarmID := gd.generateUniqueID()
	config.SetSwarmID(swarmID)
	gd.pendingContainers[swarmID] = &pendingContainer{
		Name:   swarmID,
		Config: config,
		Engine: engine,
	}

	logrus.Debugf("[MG]start pull image %s", u.config.Image)
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
	}
	if err := u.pullImage(authConfig); err != nil {
		return fmt.Errorf("pullImage Error:%s", err)
	}

	err = createNetworking(engine.IP, u.networkings)
	if err != nil {
		return err
	}

	lvs := make([]database.LocalVolume, len(pending.localStore))
	for i := range pending.localStore {
		lvs[i] = pending.localStore[i].lv
		_, err := createVolume(engine, lvs[i])
		if err != nil {
			return err
		}
	}

	container, err := engine.Create(config, swarmID, false, authConfig)
	if err != nil {
		return err
	}

	delete(gd.pendingContainers, swarmID)

	logrus.Debug("starting Containers")
	if err := engine.StartContainer(container.ID, nil); err != nil {
		return err
	}

	logrus.Debug("copy Service Config")
	if err := copyConfigIntoCNFVolume(u, lvs, u.parent.Content); err != nil {
		return err
	}

	logrus.Debug("init & Start Service")
	err = initService(container.ID, engine, u.InitServiceCmd())
	if err != nil {
		return err
	}

	// remove old container
	err = u.container.Engine.RemoveContainer(u.container, true, true)
	if err != nil {
		return err
	}

	err = engine.RenameContainer(container, u.Name)
	if err != nil {
		return err
	}

	container, err = container.Refresh()
	if err != nil {
		return err
	}

	u.container = container
	u.ContainerID = container.ID

	// update database :tb_unit
	// remove old LocalVolume
	// dealwith errors

	return nil
}

func (gd *Gardener) selectEngine(config *cluster.ContainerConfig, module structs.Module, engines, exclude []string) (*cluster.Engine, error) {
	entry := logrus.WithFields(logrus.Fields{"Module": module.Type})

	num, _type := 1, module.Type
	// TODO:maybe remove tag
	if module.Type == _SwitchManagerType {
		_type = _ProxyType
	}
	filters := gd.listShortIdleStore(module.Stores, _type, num)
	filters = append(filters, exclude...)
	entry.Debugf("[MG] %s,%s,%s:first filters of storage:%s", module.Stores, module.Type, num, filters)
	nodes := make([]*node.Node, 0, len(engines))
	for i := range engines {
		if isStringExist(engines[i], filters) {
			continue
		}
		node := gd.checkNode(engines[i])
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	logrus.Debugf("filters num:%d,candidate nodes num:%d", len(filters), len(nodes))

	candidates, err := gd.Scheduler(config, num, nodes, false, false)
	if err != nil {
		return nil, err
	}

	engine, ok := gd.engines[candidates[0].ID]
	if !ok {
		err = fmt.Errorf("Not Found Engine %s", candidates[0].ID)
		logrus.Error(err)
		return nil, err
	}

	return engine, nil
}

func resetContainerConfig(config *cluster.ContainerConfig, hostConfig *ctypes.HostConfig) (*cluster.ContainerConfig, error) {
	clone := cloneContainerConfig(config)
	//
	if hostConfig != nil {
		if hostConfig.CpusetCpus != "" {
			clone.HostConfig.CpusetCpus = hostConfig.CpusetCpus
		}
		if hostConfig.Memory != 0 {
			clone.HostConfig.Memory = hostConfig.Memory
		}
	} else {
		// reset CpusetCpus
		ncpu, err := utils.GetCPUNum(config.HostConfig.CpusetCpus)
		if err != nil {
			return nil, err
		}
		clone.HostConfig.CpusetCpus = strconv.FormatInt(ncpu, 10)
	}

	return clone, nil
}

func (gd *Gardener) UnitMigrate(name string, candidates []string, hostConfig *ctypes.HostConfig) error {
	table, err := database.GetUnit(name)
	if err != nil {
		return fmt.Errorf("Not Found Unit %s,error:%s", name, err)
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return err
	}

	svc.RLock()
	index, module := 0, structs.Module{}
	filters := make([]string, len(svc.units))
	for i, u := range svc.units {
		filters[i] = u.EngineID
		if u.Name == name {
			index = i
		}
	}
	u := svc.units[index]

	for i := range svc.base.Modules {
		if u.Type == svc.base.Modules[i].Type {
			module = svc.base.Modules[i]
			break
		}
	}
	svc.RUnlock()
	if err != nil {
		return err
	}

	err = u.stopContainer(0)
	if err != nil {
		return err
	}
	err = removeNetworking(u.engine.IP, u.networkings)
	if err != nil {
		return err
	}
	// sdk.SanDeActivate
	// delMapping

	config, err := resetContainerConfig(u.container.Config, hostConfig)
	if err != nil {
		return err
	}

	if len(candidates) > 0 {
		candidates, err = database.ListEnginesByNodes(candidates)
		if err != nil {
			return err
		}
	} else {
		table, err := database.GetNode(u.EngineID)
		if err != nil {
			return err
		}
		nodes, err := database.ListNodeByCluster(table.ClusterID)
		if err != nil {
			return err
		}
		candidates = make([]string, len(nodes))
		for i := range nodes {
			if nodes[i].EngineID != u.EngineID {
				candidates[i] = nodes[i].EngineID
			}
		}
	}

	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	engine, err := gd.selectEngine(config, module, candidates, filters)
	if err != nil {
		return err
	}
	u.engine = engine
	u.EngineID = engine.ID

	ncpu, err := parseCpuset(config.HostConfig.CpusetCpus)
	if err != nil {
		return err
	}
	cpuset, err := gd.allocCPUs(engine, ncpu)
	if err != nil {
		logrus.Errorf("Alloc CPU %d Error:%s", ncpu, err)
		return err
	}
	config.HostConfig.CpusetCpus = cpuset

	_, node, err := gd.GetNode(engine.ID)
	if err != nil {
		err := fmt.Errorf("Not Found Node %s,Error:%s", engine.Name, err)
		logrus.Error(err)

		return err
	}

	pending := pendingAllocStore{
		localStore: make([]localVolume, 0, len(module.Stores)),
		sanStore:   make([]string, 0, 3),
	}
	for i := range module.Stores {
		name := fmt.Sprintf("%s_%s_LV", u.Unit.Name, module.Stores[i].Name)

		if !store.IsLocalStore(module.Stores[i].Type) {
			continue
		}
		lv, err := node.localStorageAlloc(name, u.Unit.ID, module.Stores[i].Type, module.Stores[i].Size)
		if err != nil {
			return err
		}
		pending.localStore = append(pending.localStore, localVolume{
			lv:   lv,
			size: module.Stores[i].Size,
		})
	}

	swarmID := gd.generateUniqueID()
	config.SetSwarmID(swarmID)
	gd.pendingContainers[swarmID] = &pendingContainer{
		Name:   swarmID,
		Config: config,
		Engine: engine,
	}

	logrus.Debugf("[MG]start pull image %s", u.config.Image)
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
	}
	if err := u.pullImage(authConfig); err != nil {
		return fmt.Errorf("pullImage Error:%s", err)
	}

	err = createNetworking(engine.IP, u.networkings)
	if err != nil {
		return err
	}

	// migrate san volumes
	// Mapping LVs
	// SanVgCreate
	// SanActivate

	lvs := make([]database.LocalVolume, len(pending.localStore))
	for i := range pending.localStore {
		lvs[i] = pending.localStore[i].lv
		_, err := createVolume(engine, lvs[i])
		if err != nil {
			return err
		}
	}

	container, err := engine.Create(config, swarmID, false, authConfig)
	if err != nil {
		return err
	}

	delete(gd.pendingContainers, swarmID)

	logrus.Debug("starting Containers")
	if err := engine.StartContainer(container.ID, nil); err != nil {
		return err
	}

	logrus.Debug("copy Service Config")
	if err := copyConfigIntoCNFVolume(u, lvs, u.parent.Content); err != nil {
		return err
	}

	logrus.Debug("init & Start Service")
	err = initService(container.ID, engine, u.InitServiceCmd())
	if err != nil {
		return err
	}

	// remove old container
	err = u.container.Engine.RemoveContainer(u.container, true, true)
	if err != nil {
		return err
	}

	err = engine.RenameContainer(container, u.Name)
	if err != nil {
		return err
	}

	container, err = container.Refresh()
	if err != nil {
		return err
	}

	u.container = container
	u.ContainerID = container.ID

	// update database :tb_unit
	// remove old LocalVolume
	// dealwith errors

	return nil
}
