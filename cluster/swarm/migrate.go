package swarm

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	ctypes "github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/utils"
)

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
	oldContainer := u.container

	for i := range svc.base.Modules {
		if u.Type == svc.base.Modules[i].Type {
			module = svc.base.Modules[i]
			break
		}
	}
	svc.RUnlock()

	dc, err := gd.DatacenterByEngine(u.EngineID)
	if err != nil || dc == nil {
		return err
	}
	nodes, err := database.ListNodeByCluster(dc.ID)
	if err != nil {
		return err
	}
	out := make([]string, 0, len(nodes))
	for i := range candidates {
		if node := dc.getNode(candidates[i]); node != nil {
			out = append(out, node.EngineID)
			continue
		}
		for n := range nodes {
			if nodes[n].ID == candidates[i] ||
				nodes[n].Name == candidates[i] ||
				nodes[n].EngineID == candidates[i] {
				out = append(out, nodes[n].EngineID)
				break
			}
		}
	}
	if len(out) == 0 {
		for i := range nodes {
			if nodes[i].EngineID != u.EngineID {
				out = append(out, nodes[i].EngineID)
			}
		}
	}

	config, err := resetContainerConfig(u.container.Config, hostConfig)
	if err != nil {
		return err
	}
	gd.scheduler.Lock()
	defer gd.scheduler.Unlock()

	engine, err := gd.selectEngine(config, module, out, filters)
	if err != nil {
		return err
	}

	err = svc.isolate(u.Name)
	if err != nil {
		// return err
	}

	err = u.stopContainer(0)
	if err != nil {
		return err
	}
	err = removeNetworking(u.engine.IP, u.networkings)
	if err != nil {
		return err
	}

	// local volumes
	oldLVs, err := database.SelectVolumesByUnitID(u.ID)
	if err != nil {
		return err
	}

	lunMap := make(map[string][]database.LUN, len(oldLVs))
	lunSlice := make([]database.LUN, 0, len(oldLVs))
	for i := range oldLVs {
		vg := oldLVs[i].VGName
		if val, ok := lunMap[vg]; ok && len(val) > 0 {
			continue
		}
		if isSanVG(vg) {
			list, err := database.ListLUNByVgName(vg)
			if err != nil {
				return err
			}
			if len(list) > 0 {
				lunMap[vg] = list
				lunSlice = append(lunSlice, list...)
			}
		}
	}

	if len(lunMap) > 0 {
		if dc.storage == nil {
			return fmt.Errorf("%s storage error", dc.Name)
		}

		for vg, list := range lunMap {
			names := make([]string, len(list))
			hostLuns := make([]int, len(list))
			for i := range list {
				names[i] = list[i].Name
				hostLuns[i] = list[i].HostLunID
			}
			config := sdk.DeactivateConfig{
				VgName:    vg,
				Lvname:    names,
				HostLunId: hostLuns,
				Vendor:    dc.storage.Vendor(),
			}
			// san volumes
			err = u.deactivateVG(config)
			if err != nil {
				return err
			}
		}

		for i := range lunSlice {
			err := dc.storage.DelMapping(lunSlice[i].ID)
			if err != nil {
				logrus.Errorf("%s DelMapping %s", dc.storage.Vendor(), lunSlice[i].ID)
			}
		}
	}

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
	for i := range lunSlice {
		err = dc.storage.Mapping(node.ID, lunSlice[i].VGName, lunSlice[i].ID)
		if err != nil {
			return err
		}
	}
	// SanVgCreate
	for vg, list := range lunMap {
		l, size := make([]int, len(list)), 0

		for i := range list {
			l[i] = list[i].StorageLunID
			size += list[i].SizeByte
		}

		config := sdk.VgConfig{
			HostLunId: l,
			VgName:    vg,
			Type:      dc.storage.Vendor(),
		}

		addr := getPluginAddr(engine.IP, pluginPort)
		err := sdk.SanVgCreate(addr, config)
		if err != nil {
			return err
		}
	}

	// SanActivate
	for vg, list := range lunMap {
		names := make([]string, len(list))
		for i := range list {
			names[i] = list[i].Name
		}
		activeConfig := sdk.ActiveConfig{
			VgName: vg,
			Lvname: names,
		}
		err := sdk.SanActivate(getPluginAddr(engine.IP, pluginPort), activeConfig)
		if err != nil {
			return err
		}
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

	// remove old LocalVolume
	for i := range oldLVs {
		err := oldContainer.Engine.RemoveVolume(oldLVs[i].Name)
		if err != nil {
			logrus.Errorf("%s remove old volume %s", u.Name, oldLVs[i].Name)
			return err
		}
	}

	// remove old container
	err = oldContainer.Engine.RemoveContainer(oldContainer, true, true)
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
	u.engine = engine
	u.EngineID = engine.ID
	u.CreatedAt = time.Now()

	// update database :tb_unit
	// delete old localVolumes
	tx, err := database.GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i := range oldLVs {
		if isSanVG(oldLVs[i].VGName) {
			continue
		}
		err := database.TxDeleteVolume(tx, oldLVs[i].ID)
		if err != nil {
			return err
		}
	}
	err = database.TxUpdateUnit(tx, u.Unit)
	if err != nil {
		return err
	}

	// switchback unit
	err = svc.switchBack(u.Name)
	if err != nil {
	}

	// dealwith errors

	return err
}
