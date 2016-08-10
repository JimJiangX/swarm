package swarm

import (
	"fmt"
	"strconv"
	"strings"
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
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (gd *Gardener) selectEngine(config *cluster.ContainerConfig, module structs.Module, list []database.Node, exclude []string) (*cluster.Engine, error) {
	entry := logrus.WithFields(logrus.Fields{"Module": module.Type})

	num, _type := 1, module.Type
	// TODO:maybe remove tag
	if module.Type == _SwitchManagerType {
		_type = _ProxyType
	}

	list = gd.shortIdleStoreFilter(list, module.Stores, _type, num)

	entry.Debugf("[MG] %v,%s,%d:after filters of storage:%d", module.Stores, module.Type, num, len(list))

	nodes := make([]*node.Node, 0, len(list))
	for i := range list {
		if isStringExist(list[i].EngineID, exclude) {
			continue
		}
		node := gd.checkNode(list[i].EngineID)
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	logrus.Debugf("filters num:%d,candidate nodes num:%d", len(exclude), len(nodes))

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

func (dc *Datacenter) listCandidates(candidates []string) ([]database.Node, error) {
	nodes, err := database.ListNodeByCluster(dc.ID)
	if err != nil {
		return nil, err
	}

	out := make([]database.Node, 0, len(nodes))

	for i := range candidates {
		if node := dc.getNode(candidates[i]); node != nil {
			out = append(out, *node.Node)
			continue
		}
		for n := range nodes {
			if nodes[n].ID == candidates[i] ||
				nodes[n].Name == candidates[i] ||
				nodes[n].EngineID == candidates[i] {
				out = append(out, *nodes[n])
				break
			}
		}
	}
	if len(out) == 0 {
		for i := range nodes {
			out = append(out, *nodes[i])
		}
	}

	return out, nil
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

	clone.HostConfig.Binds = make([]string, 0, 5)

	return clone, nil
}

func (gd *Gardener) UnitMigrate(nameOrID string, candidates []string, hostConfig *ctypes.HostConfig) (string, error) {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return "", fmt.Errorf("Not Found Unit %s,error:%s", nameOrID, err)
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return "", err
	}

	sys, err := gd.SystemConfig()
	if err != nil {
		return "", err
	}

	svc.RLock()

	migrate, err := svc.getUnit(table.ID)
	if migrate == nil || err != nil {
		svc.RUnlock()

		logrus.Warn(err)
		svc, err = gd.rebuildService(table.ServiceID)
		if err != nil {
			return "", err
		}

		svc.RLock()

		migrate, err = svc.getUnit(table.ID)
		if migrate == nil || err != nil {
			logrus.Error(err)
			svc.RUnlock()

			return "", err
		}
	}

	oldContainer := migrate.container

	filters := make([]string, len(svc.units))
	for i, u := range svc.units {
		filters[i] = u.EngineID
	}

	module := structs.Module{}
	san := false
	for i := range svc.base.Modules {
		if migrate.Type == svc.base.Modules[i].Type {
			module = svc.base.Modules[i]
			for s := range module.Stores {
				if strings.ToUpper(module.Stores[s].Type) == "SAN" {
					san = true
				}
			}
			break
		}
	}

	svc.RUnlock()

	if !san && migrate.Type == _UpsqlType {
		return "", errors.Errorf("Unit %s storage hasn't SAN Storage,Cannot Exec Migrate", nameOrID)
	}

	dc, original, err := gd.GetNode(migrate.EngineID)
	if err != nil || dc == nil {
		return "", err
	}

	out, err := dc.listCandidates(candidates)
	if err != nil {
		return "", err
	}
	logrus.Debugf("listCandidates:%d", len(out))

	config, err := resetContainerConfig(migrate.container.Config, hostConfig)
	if err != nil {
		return "", err
	}

	gd.scheduler.Lock()

	engine, err := gd.selectEngine(config, module, out, filters)
	if err != nil {
		gd.scheduler.Unlock()
		return "", err
	}
	gd.scheduler.Unlock()

	background := func(ctx context.Context) (err error) {
		pending := newPendingAllocResource()

		svc.Lock()
		gd.scheduler.Lock()

		defer func() {
			if err == nil {
				migrate.Status, migrate.LatestError = statusUnitMigrated, ""
			} else {
				migrate.Status, migrate.LatestError = statusUnitMigrateFailed, err.Error()
			}

			gd.scheduler.Unlock()
			if err != nil {
				logrus.Error(err)
				// error handle
				_err := gd.Recycle([]*pendingAllocResource{pending})
				if _err != nil {
					logrus.Error("Recycle ", _err)
				}
			}

			svc.Unlock()
		}()

		networkings, err := getIPInfoByUnitID(migrate.ID, engine)
		if err != nil {
			return err
		}

		cpuset, err := gd.allocCPUs(engine, config.HostConfig.CpusetCpus)
		if err != nil {
			logrus.Errorf("Alloc CPU '%s' Error:%s", config.HostConfig.CpusetCpus, err)
			return err
		}
		config.HostConfig.CpusetCpus = cpuset

		if migrate.Type != _SwitchManagerType {
			err := svc.isolate(migrate.Name)
			if err != nil {
				logrus.Errorf("isolate container %s error:%s", migrate.Name, err)
			}
		}

		err = stopOldContainer(svc, migrate)
		if err != nil {
			return err
		}

		oldLVs, lunMap, lunSlice, err := listOldVolumes(migrate.ID)
		if err != nil {
			return err
		}

		if len(lunMap) > 0 {
			err = sanDeactivateAndDelMapping(dc.storage, original.engine.IP, lunMap, lunSlice)
			if err != nil {
				return err
			}
		}

		defer func() {
			if err != nil {
				_, err := migrateVolumes(dc.storage, original.ID, original.engine, oldLVs, oldLVs, lunMap, lunSlice)
				if err != nil {
					logrus.Error(err)
					//	return err
				}
				return
			}

			logrus.Debug("recycle old container volumes resource")

			// clean local volumes
			for i := range oldLVs {
				if isSanVG(oldLVs[i].VGName) {
					continue
				}
				_err := original.localStore.Recycle(oldLVs[i].ID)
				if err != nil {
					logrus.Error(_err)
				}
			}
		}()

		pending.unit = migrate
		pending.engine = engine

		err = gd.allocStorage(pending, engine, config, module.Stores, true)
		if err != nil {
			return err
		}

		swarmID := gd.generateUniqueID()
		config.SetSwarmID(swarmID)
		pending.pendingContainer = &pendingContainer{
			Name:   swarmID,
			Config: config,
			Engine: engine,
		}

		gd.pendingContainers[swarmID] = pending.pendingContainer

		logrus.Debugf("[MG]start pull image %s", config.Image)

		dc, node, err := gd.GetNode(engine.ID)
		if err != nil {
			err := fmt.Errorf("Not Found Node %s,Error:%s", engine.Name, err)
			logrus.Error(err)

			return err
		}

		lvs, err := migrateVolumes(dc.storage, node.ID, engine, pending.localStore, oldLVs, lunMap, lunSlice)
		if err != nil {
			return err
		}

		if svc.authConfig == nil {
			svc.authConfig, err = gd.RegistryAuthConfig()
			if err != nil {
				return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
			}
		}
		container, err := engine.CreateContainer(config, swarmID, true, svc.authConfig)
		if err != nil {
			return err
		}

		defer func(c *cluster.Container, addr string,
			networkings []IPInfo, lvs []database.LocalVolume) {

			if err == nil {
				return
			}
			logrus.Debugf("clean new container %s", c.ID)

			_err := cleanOldContainer(c, lvs)
			if _err != nil {
				logrus.Error(_err)
			}
			err = removeNetworkings(addr, networkings)
			if err != nil {
				logrus.Errorf("container %s remove Networkings error:%s", migrate.Name, err)
			}
		}(container, engine.IP, networkings, pending.localStore)

		err = startUnit(engine, container.ID, migrate, networkings, lvs)
		delete(gd.pendingContainers, swarmID)

		if err != nil {
			return err
		}

		err = engine.RenameContainer(container, migrate.Name)
		if err != nil {
			logrus.Error(err)

			return err
		}

		logrus.WithFields(logrus.Fields{
			"Engine":    engine.Addr,
			"Container": container.ID,
			"NewName":   container.Names,
		}).Debug("Rename Container")

		migrate.container = container
		migrate.ContainerID = container.ID
		migrate.config = container.Config
		migrate.engine = engine
		migrate.EngineID = engine.ID
		migrate.networkings = networkings
		migrate.CreatedAt = time.Now()

		err = updateUnit(migrate.Unit, oldLVs, false)
		if err != nil {
			logrus.Errorf("updateUnit in database error:%s", err)

			return err
		}

		err = gd.SaveContainerToConsul(container)
		if err != nil {
			logrus.Errorf("Save Container To Consul error:%s", err)
			// return err
		}

		oldEngineIP := oldContainer.Engine.IP
		err = cleanOldContainer(oldContainer, oldLVs)
		if err != nil {
			logrus.Error(err)
		}

		err = deregisterToServices(oldEngineIP, migrate.ID, sys)
		if err != nil {
			logrus.Error(err)
		}

		err = registerToServers(migrate, svc, sys)
		if err != nil {
			logrus.Errorf("registerToServers error:%s", err)
		}

		if migrate.Type != _SwitchManagerType {
			// switchback unit
			err = svc.switchBack(migrate.Name)
			if err != nil {
				logrus.Errorf("switchBack error:%s", err)
			}
		}

		return nil
	}

	task := database.NewTask(migrate.Name, _Unit_Migrate_Task, migrate.ID, "", nil, 0)

	create := func() error {
		migrate.Status, migrate.LatestError = statusUnitMigrating, ""

		return database.TxUpdateUnitAndInsertTask(&migrate.Unit, task)
	}

	update := func(code int, msg string) error {
		task.Status = int64(code)

		return database.TxUpdateUnitStatusWithTask(&migrate.Unit, &task, msg)
	}

	t := NewAsyncTask(context.Background(), background, create, update, 0)

	return task.ID, t.Run()
}

func startUnit(engine *cluster.Engine, containerID string,
	u *unit, networkings []IPInfo, lvs []database.LocalVolume) error {

	err := startContainer(containerID, engine, networkings)
	if err != nil {
		return err
	}

	logrus.Debug("copy Service Config")
	err = copyConfigIntoCNFVolume(engine.IP, u.Path(), u.parent.Content, lvs)
	if err != nil {
		return err
	}

	logrus.Debug("init & Start Service")
	err = initUnitService(containerID, engine, u.InitServiceCmd())
	if err != nil {
		logrus.Error(err)
	}

	return err
}

func stopOldContainer(svc *Service, u *unit) error {
	err := u.forceStopService()
	if err != nil {
		logrus.Errorf("container %s stop service error:%s", u.Name, err)

		err1 := checkContainerError(err)
		if err.Error() != "EOF" && err1 != errContainerNotFound || err1 != errContainerNotRunning {
			return err
		}
	}

	if err = u.forceStopContainer(0); err != nil {
		logrus.Errorf("%s stop container error:%s", u.Name, err)
		err1 := checkContainerError(err)
		if err1 != errContainerNotRunning && err1 != errContainerNotFound {
			return err
		}
	}

	err = removeNetworkings(u.engine.IP, u.networkings)
	if err != nil {
		logrus.Errorf("container %s remove Networkings error:%s", u.Name, err)
	}
	return err
}

func sanDeactivateAndDelMapping(storage store.Store, host string,
	lunMap map[string][]database.LUN, lunSlice []database.LUN) error {
	if storage == nil {
		return fmt.Errorf("Store is nil")
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
			Vendor:    storage.Vendor(),
		}
		// san volumes
		addr := getPluginAddr(host, pluginPort)
		err := sdk.SanDeActivate(addr, config)
		if err != nil {
			logrus.Error("%s SanDeActivate error:%s", host, err)
		}
	}

	for i := range lunSlice {
		err := storage.DelMapping(lunSlice[i].ID)
		if err != nil {
			logrus.Errorf("%s DelMapping %s", storage.Vendor(), lunSlice[i].ID)
		}
	}

	return nil
}

func listOldVolumes(unit string) ([]database.LocalVolume, map[string][]database.LUN, []database.LUN, error) {
	// local volumes
	lvs, err := database.SelectVolumesByUnitID(unit)
	if err != nil {
		return nil, nil, nil, err
	}

	lunMap := make(map[string][]database.LUN, len(lvs))
	lunSlice := make([]database.LUN, 0, len(lvs))
	for i := range lvs {
		vg := lvs[i].VGName
		if val, ok := lunMap[vg]; ok && len(val) > 0 {
			continue
		}
		if isSanVG(vg) {
			out, err := database.ListLUNByVgName(vg)
			if err != nil {
				return nil, nil, nil, err
			}
			if len(out) > 0 {
				lunMap[vg] = out
				lunSlice = append(lunSlice, out...)
			}
		}
	}

	return lvs, lunMap, lunSlice, nil
}

// migrate san volumes
// create local volumes
func migrateVolumes(storage store.Store, nodeID string,
	engine *cluster.Engine,
	localStore, oldLVs []database.LocalVolume,
	lunMap map[string][]database.LUN,
	lunSlice []database.LUN) ([]database.LocalVolume, error) {

	// Mapping LVs
	if len(lunSlice) > 0 && storage == nil {
		return nil, fmt.Errorf("Store is nil")
	}

	for i := range lunSlice {
		err := storage.Mapping(nodeID, lunSlice[i].VGName, lunSlice[i].ID)
		if err != nil {
			return nil, err
		}
	}

	vgMap := make(map[string][]database.LUN, len(lunMap))
	for vg := range lunMap {
		if isSanVG(vg) {
			out, err := database.ListLUNByVgName(vg)
			if err != nil {
				return nil, err
			}
			if len(out) > 0 {
				vgMap[vg] = out
			}
		}
	}

	addr := getPluginAddr(engine.IP, pluginPort)
	// SanActivate
	for vg, list := range vgMap {
		names := make([]string, len(list))
		for i := range list {
			names[i] = list[i].Name
		}
		activeConfig := sdk.ActiveConfig{
			VgName: vg,
			Lvname: names,
		}
		err := sdk.SanActivate(addr, activeConfig)
		if err != nil {
			return nil, err
		}
	}

	// SanVgCreate
	for vg, list := range vgMap {
		l, size := make([]int, len(list)), 0

		for i := range list {
			l[i] = list[i].HostLunID
			size += list[i].SizeByte
		}

		config := sdk.VgConfig{
			HostLunId: l,
			VgName:    vg,
			Type:      storage.Vendor(),
		}

		err := sdk.SanVgCreate(addr, config)
		if err != nil {
			return nil, err
		}
	}

	lvs := make([]database.LocalVolume, len(localStore), len(oldLVs))
	copy(lvs, localStore)
	for i := range localStore {
		_, err := createVolume(engine, localStore[i])
		if err != nil {
			return lvs, err
		}
	}

	for i := range oldLVs {
		if isSanVG(oldLVs[i].VGName) {
			lvs = append(lvs, oldLVs[i])
			_, err := createVolume(engine, oldLVs[i])
			if err != nil {
				return lvs, err
			}
		}
	}

	return lvs, nil
}

func cleanOldContainer(old *cluster.Container, lvs []database.LocalVolume) error {
	engine := old.Engine
	if engine == nil {
		return errEngineIsNil
	}

	logrus.Debugf("Engine %s remove container %s", engine.Addr, old.ID)

	// remove old container
	err := engine.RemoveContainer(old, true, true)
	engine.CheckConnectionErr(err)
	if err != nil {
		logrus.Errorf("engine %s remove container %s error:%s", engine.Addr, old.Info.Name, err)
	}

	// remove old LocalVolume
	for i := range lvs {
		err := engine.RemoveVolume(lvs[i].Name)
		if err != nil {
			logrus.Errorf("%s remove old volume %s", old.Info.Name, lvs[i].Name)
			return err
		}
		logrus.Debugf("Engine %s remove volume %s", engine.Addr, lvs[i].Name)
	}

	return nil
}

func updateUnit(unit database.Unit, lvs []database.LocalVolume, reserveSAN bool) error {
	// update database :tb_unit
	// delete old localVolumes
	tx, err := database.GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i := range lvs {
		if reserveSAN && isSanVG(lvs[i].VGName) {
			continue
		}
		err := database.TxDeleteVolume(tx, lvs[i].ID)
		if err != nil {
			logrus.Error(err)
		}
	}
	err = database.TxUpdateUnit(tx, unit)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (gd *Gardener) UnitRebuild(nameOrID string, candidates []string, hostConfig *ctypes.HostConfig) (string, error) {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return "", fmt.Errorf("Not Found Unit %s,error:%s", nameOrID, err)
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return "", err
	}

	sys, err := gd.SystemConfig()
	if err != nil {
		return "", err
	}

	svc.RLock()

	rebuild, err := svc.getUnit(table.ID)
	if rebuild == nil || err != nil {
		svc.RUnlock()

		logrus.Warn(err)
		svc, err = gd.rebuildService(table.ServiceID)
		if err != nil {
			return "", err
		}

		svc.RLock()

		rebuild, err = svc.getUnit(table.ID)
		if rebuild == nil || err != nil {
			logrus.Error(err)
			svc.RUnlock()

			return "", err
		}
	}

	oldContainer := rebuild.container

	filters := make([]string, len(svc.units))
	for i, u := range svc.units {
		filters[i] = u.EngineID
	}

	module := structs.Module{}
	for i := range svc.base.Modules {
		if rebuild.Type == svc.base.Modules[i].Type {
			module = svc.base.Modules[i]
			break
		}
	}

	svc.RUnlock()

	dc, original, err := gd.GetNode(rebuild.EngineID)
	if err != nil || dc == nil {
		return "", err
	}

	out, err := dc.listCandidates(candidates)
	if err != nil {
		return "", err
	}
	logrus.Debugf("listCandidates:%d", len(out))

	config, err := resetContainerConfig(rebuild.container.Config, hostConfig)
	if err != nil {
		return "", err
	}

	gd.scheduler.Lock()

	engine, err := gd.selectEngine(config, module, out, filters)
	if err != nil {
		gd.scheduler.Unlock()

		return "", err
	}
	gd.scheduler.Unlock()

	background := func(ctx context.Context) (err error) {
		pending := newPendingAllocResource()

		svc.Lock()
		gd.scheduler.Lock()

		defer func() {
			if err == nil {
				rebuild.Status, rebuild.LatestError = statusUnitRebuilt, ""
			} else {
				rebuild.Status, rebuild.LatestError = statusUnitRebuildFailed, err.Error()
			}

			gd.scheduler.Unlock()
			if err != nil {
				logrus.Error(err)
				// error handle
				_err := gd.Recycle([]*pendingAllocResource{pending})
				if _err != nil {
					logrus.Error("Recycle ", _err)
				}
			}

			svc.Unlock()
		}()

		networkings, err := getIPInfoByUnitID(rebuild.ID, engine)
		if err != nil {
			return err
		}

		cpuset, err := gd.allocCPUs(engine, config.HostConfig.CpusetCpus)
		if err != nil {
			logrus.Errorf("Alloc CPU '%s' Error:%s", config.HostConfig.CpusetCpus, err)
			return err
		}
		config.HostConfig.CpusetCpus = cpuset

		oldLVs, lunMap, lunSlice, err := listOldVolumes(rebuild.ID)
		if err != nil {
			return err
		}

		defer func() {
			if err != nil {
				return
			}
			// deactivate
			// del mapping
			if len(lunMap) > 0 {
				// TODO:fix host
				_err := sanDeactivateAndDelMapping(dc.storage, original.engine.IP, lunMap, lunSlice)
				if _err != nil {
					logrus.Error(_err)
				}
			}

			logrus.Debug("recycle old container volumes resource")
			// recycle lun
			for i := range lunSlice {
				_err := dc.storage.Recycle(lunSlice[i].ID, 0)
				if _err != nil {
					logrus.Error(_err)
				}
			}

			// clean local volumes
			for i := range oldLVs {
				if isSanVG(oldLVs[i].VGName) {
					continue
				}
				_err := original.localStore.Recycle(oldLVs[i].ID)
				if err != nil {
					logrus.Error(_err)
				}
			}

			// clean database
			_err := database.TxDeleteVolumes(oldLVs)
			if _err != nil {
				logrus.Error(_err)
			}
		}()

		pending.unit = rebuild
		pending.engine = engine

		err = gd.allocStorage(pending, engine, config, module.Stores, false)
		if err != nil {
			return err
		}

		swarmID := gd.generateUniqueID()
		config.SetSwarmID(swarmID)
		pending.pendingContainer = &pendingContainer{
			Name:   swarmID,
			Config: config,
			Engine: engine,
		}

		gd.pendingContainers[swarmID] = pending.pendingContainer

		err = createServiceResources(gd, []*pendingAllocResource{pending})
		if err != nil {
			logrus.Errorf("create Service Resources error:%s", err)
			return err
		}

		if svc.authConfig == nil {
			svc.authConfig, err = gd.RegistryAuthConfig()
			if err != nil {
				return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
			}
		}

		logrus.Debugf("Engine %s create container %s", engine.Addr, swarmID)
		container, err := engine.CreateContainer(config, swarmID, true, svc.authConfig)
		delete(gd.pendingContainers, swarmID)

		if err != nil {
			return err
		}

		defer func(c *cluster.Container, addr string,
			networkings []IPInfo, lvs []database.LocalVolume) {

			if err == nil {
				return
			}
			logrus.Debugf("clean new container %s", c.ID)

			_err := cleanOldContainer(c, lvs)
			if _err != nil {
				logrus.Error(_err)
			}
			err = removeNetworkings(addr, networkings)
			if err != nil {
				logrus.Errorf("container %s remove Networkings error:%s", rebuild.Name, err)
			}

		}(container, engine.IP, networkings, pending.localStore)

		err = startUnit(engine, container.ID, rebuild, networkings, pending.localStore)
		if err != nil {
			return err
		}

		if rebuild.Type != _SwitchManagerType {
			err := svc.isolate(rebuild.Name)
			if err != nil {
				logrus.Errorf("isolate container %s error:%s", rebuild.Name, err)
			}
		}

		err = stopOldContainer(svc, rebuild)
		if err != nil {
			logrus.Error(err)
			// return err
		}

		err = engine.RenameContainer(container, rebuild.Name)
		if err != nil {
			logrus.Error(err)

			return err
		}

		rebuild.container = container
		rebuild.ContainerID = container.ID
		rebuild.config = container.Config
		rebuild.engine = engine
		rebuild.EngineID = engine.ID
		rebuild.networkings = networkings
		rebuild.CreatedAt = time.Now()

		err = updateUnit(rebuild.Unit, oldLVs, true)
		if err != nil {
			logrus.Errorf("updateUnit in database error:%s", err)
			return err
		}

		err = gd.SaveContainerToConsul(container)
		if err != nil {
			logrus.Errorf("Save Container To Consul error:%s", err)
			// return err
		}

		oldEngineIP := oldContainer.Engine.IP
		err = cleanOldContainer(oldContainer, oldLVs)
		if err != nil {
			logrus.Error(err)
		}

		err = deregisterToServices(oldEngineIP, rebuild.ID, sys)
		if err != nil {
			logrus.Error(err)
		}

		err = registerToServers(rebuild, svc, sys)
		if err != nil {
			logrus.Errorf("registerToServers error:%s", err)
		}

		if rebuild.Type != _SwitchManagerType {
			// switchback unit
			err = svc.switchBack(rebuild.Name)
			if err != nil {
				logrus.Errorf("switchBack error:%s", err)
			}
		}

		return nil
	}

	task := database.NewTask(rebuild.Name, _Unit_Rebuild_Task, rebuild.ID, "", nil, 0)

	create := func() error {
		rebuild.Status, rebuild.LatestError = statusUnitRebuilding, ""

		return database.TxUpdateUnitAndInsertTask(&rebuild.Unit, task)
	}

	update := func(code int, msg string) error {
		task.Status = int64(code)

		return database.TxUpdateUnitStatusWithTask(&rebuild.Unit, &task, msg)
	}

	t := NewAsyncTask(context.Background(), background, create, update, 0)

	return task.ID, t.Run()
}

func registerToServers(u *unit, svc *Service, sys database.Configurations) error {
	logrus.Debug("[MG]register Services")
	if err := registerHealthCheck(u, sys.ConsulConfig, svc); err != nil {
		logrus.Error(err)
	}

	logrus.Debug("[MG]register To Horus")
	obj, err := u.registerHorus(sys.MonitorUsername, sys.MonitorPassword, sys.HorusAgentPort)
	if err != nil {
		err = fmt.Errorf("container %s register Horus Error:%s", u.Name, err)
		logrus.Error(err)

		return err
	}

	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	err = registerToHorus(horus, []registerService{obj})
	if err != nil {
		logrus.Errorf("registerToHorus error:%s", err)
	}

	return err
}

func deregisterToServices(addr, unitID string, sys database.Configurations) error {
	configs := sys.GetConsulConfigs()
	if len(configs) == 0 {
		return fmt.Errorf("GetConsulConfigs error %v %v", configs[0])
	}

	logrus.Debugf("deregister HealthCheck %s", unitID)

	err := deregisterHealthCheck(addr, unitID, configs[0])
	if err != nil {
		logrus.Error(err)
	}

	logrus.Debugf("deregister Horus %s", unitID)

	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	endpoints := []deregisterService{{unitID}}

	err = deregisterToHorus(horus, endpoints, false)
	if err != nil {
		logrus.WithField("Endpoints", endpoints).Errorf("Deregister To Horus:%s", horus)

		err = deregisterToHorus(horus, endpoints, true)
		if err != nil {
			logrus.WithField("Endpoints", endpoints).Errorf("Deregister To Horus:%s,force=true", horus)
			return err
		}
	}

	return err
}
