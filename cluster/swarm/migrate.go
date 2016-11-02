package swarm

import (
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	ctypes "github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/storage"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
	"github.com/upmio/local_plugin_volume/sdk"
	"golang.org/x/net/context"
)

func (gd *Gardener) selectEngine(config *cluster.ContainerConfig, module structs.Module, list []database.Node, exclude []string) (*cluster.Engine, error) {
	entry := logrus.WithFields(logrus.Fields{"Module": module.Type})
	num := 1

	list, err := gd.resourceFilter(list, module, num)
	if err != nil {
		return nil, err
	}

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

	entry.Debugf("filters num:%d,candidate nodes num:%d", len(exclude), len(nodes))

	candidates, err := gd.dispatch(config, num, nodes, false, false)
	if err != nil {
		return nil, err
	}

	engine, ok := gd.engines[candidates[0].ID]
	if !ok {
		err = errors.New("not found Engine:" + candidates[0].ID)
		entry.Error(err)
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
				out = append(out, nodes[n])
				break
			}
		}
	}
	if len(out) == 0 {
		out = nodes
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
			return nil, errors.Wrap(err, "get CPU num")
		}
		clone.HostConfig.CpusetCpus = strconv.FormatInt(ncpu, 10)
	}

	clone.HostConfig.Binds = make([]string, 0, 5)

	return clone, nil
}

// UnitMigrate migrate the assigned unit to another host
func (gd *Gardener) UnitMigrate(nameOrID string, candidates []string, hostConfig *ctypes.HostConfig) (string, error) {

	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return "", err
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return "", err
	}

	svc.RLock()

	migrate, err := svc.getUnit(table.ID)
	if err != nil {
		svc.RUnlock()

		svc, err = gd.reloadService(table.ServiceID)
		if err != nil {
			return "", err
		}

		svc.RLock()
		migrate, err = svc.getUnit(table.ID)
		if err != nil {
			svc.RUnlock()

			return "", err
		}
	}

	entry := logrus.WithFields(logrus.Fields{
		"Service": svc.Name,
		"Migrate": migrate.Name,
	})

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

	dc, original, err := gd.getNode(migrate.EngineID)
	if err != nil || dc == nil {
		return "", err
	}

	out, err := dc.listCandidates(candidates)
	if err != nil {
		return "", err
	}
	entry.Debugf("list Candidates:%d", len(out))

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
			if err != nil {
				entry.Errorf("%+v", err)
			}
			if err == nil {
				migrate.Status, migrate.LatestError = statusUnitMigrated, ""
			} else {
				migrate.Status, migrate.LatestError = statusUnitMigrateFailed, err.Error()
			}

			gd.scheduler.Unlock()
			if err != nil {
				entry.Errorf("Unit migrate failed,%+v", err)
				// error handle
				_err := gd.resourceRecycle([]*pendingAllocResource{pending})
				if _err != nil {
					entry.Errorf("defer resourceRecycle:%+v", _err)
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
			entry.Errorf("alloc CPU '%s' error:%s", config.HostConfig.CpusetCpus, err)
			return err
		}
		config.HostConfig.CpusetCpus = cpuset

		if migrate.Type != _SwitchManagerType {
			err := svc.isolate(migrate.Name)
			if err != nil {
				entry.Errorf("isolate container error:%+v", err)
				return err
			}
		}

		err = stopOldContainer(migrate)
		if err != nil {
			return err
		}

		oldLVs, lunMap, lunSlice, err := listOldVolumes(migrate.ID)
		if err != nil {
			return err
		}

		if len(lunMap) > 0 {
			err = sanDeactivateAndDelMapping(dc.store, original.engine.IP, lunMap, lunSlice)
			if err != nil {
				return err
			}
		}

		defer func() {
			if err != nil {
				entry.Errorf("%+v", err)

				if len(lunMap) > 0 {
					err := sanDeactivateAndDelMapping(dc.store, engine.IP, lunMap, lunSlice)
					if err != nil {
						entry.Errorf("defer san Deactivate And DelMapping,%+v", err)
					}
				}

				_, err := migrateVolumes(dc.store, original.ID, original.engine, oldLVs, oldLVs, lunMap, lunSlice)
				if err != nil {
					entry.Errorf("defer migrate volumes,%+v", err)
					//	return err
				}
				return
			}

			entry.Debug("recycle old container volumes resource")

			// clean local volumes
			for i := range oldLVs {
				if isSanVG(oldLVs[i].VGName) {
					continue
				}
				err := original.localStore.Recycle(oldLVs[i].ID)
				if err != nil {
					entry.Errorf("defer recycle local volume,%+v", err)
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

		dc, node, err := gd.getNode(engine.ID)
		if err != nil {
			return err
		}

		lvs, err := migrateVolumes(dc.store, node.ID, engine, pending.localStore, oldLVs, lunMap, lunSlice)
		if err != nil {
			return err
		}

		if svc.authConfig == nil {
			svc.authConfig, err = gd.registryAuthConfig()
			if err != nil {
				return err
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
			} else {
				entry.Errorf("%+v", err)
			}

			_err := cleanOldContainer(c, lvs)
			if _err != nil {
				entry.Errorf("defer,clean container %s,%+v", c.Info.Name, _err)
			}

			_err = removeNetworkings(addr, networkings)
			if _err != nil {
				entry.Errorf("defer,container %s remove Networkings:%+v", migrate.Name, _err)
			}

		}(container, engine.IP, networkings, pending.localStore)

		err = startUnit(engine, container.ID, migrate, networkings, lvs)
		delete(gd.pendingContainers, swarmID)

		if err != nil {
			return err
		}

		err = engine.RenameContainer(container, migrate.Name)
		if err != nil {
			return errors.Wrap(err, "rename container")
		}

		logrus.WithFields(logrus.Fields{
			"Engine":    engine.Addr,
			"Container": container.ID,
			"NewName":   migrate.Name,
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
			return err
		}

		err = saveContainerToConsul(container)
		if err != nil {
			logrus.Errorf("Save Container To Consul error:%+v", err)
			// return err
		}

		oldEngineIP := oldContainer.Engine.IP
		err = cleanOldContainer(oldContainer, oldLVs)
		if err != nil {
			logrus.Error(err)
		}

		sys, err := gd.systemConfig()
		if err != nil {
			logrus.WithError(err).Error("get System Config")
		}

		err = deregisterToServices(oldEngineIP, migrate.ID)
		err = registerToServers(migrate, svc, sys)

		if migrate.Type != _SwitchManagerType {
			// switchback unit
			err = svc.switchBack(migrate.Name)
			if err != nil {
				entry.Errorf("switchBack error:%+v", err)
			}
		}

		return nil
	}

	task := database.NewTask(migrate.Name, unitMigrateTask, migrate.ID, "", nil, 0)

	create := func() error {
		migrate.Status, migrate.LatestError = statusUnitMigrating, ""

		return database.TxUpdateUnitAndInsertTask(&migrate.Unit, task)
	}

	update := func(code int, msg string) error {
		task.Status = int64(code)

		return database.TxUpdateUnitStatusWithTask(&migrate.Unit, &task, msg)
	}

	t := NewAsyncTask(context.Background(),
		background,
		create,
		update,
		0)

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

	return err
}

func stopOldContainer(u *unit) error {
	err := removeNetworkings(u.engine.IP, u.networkings)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Unit":   u.Name,
			"Engine": u.engine.Addr,
		}).WithError(err).Error("remove Networkings")
	}

	err = u.forceStopService()
	if err != nil {
		_err := checkContainerError(err)
		if _err.Error() != "EOF" && _err != errContainerNotFound || _err != errContainerNotRunning {
			return err
		}
	}

	if err = u.forceStopContainer(0); err != nil {
		_err := checkContainerError(err)
		if _err != errContainerNotRunning && _err != errContainerNotFound {
			return err
		}
	}

	return err
}

func sanDeactivateAndDelMapping(storage storage.Store, host string,
	lunMap map[string][]database.LUN, lunSlice []database.LUN) error {
	if storage == nil {
		return errors.New("Store is required")
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
			HostLunID: hostLuns,
			Vendor:    storage.Vendor(),
		}
		// san volumes
		addr := getPluginAddr(host, pluginPort)
		err := sdk.SanDeActivate(addr, config)
		if err != nil {
			logrus.Errorf("%s SanDeActivate error:%s", host, err)
			return err
		}
	}

	for i := range lunSlice {
		err := storage.DelMapping(lunSlice[i].ID)
		if err != nil {
			logrus.Errorf("%s DelMapping %s", storage.Vendor(), lunSlice[i].ID)
			return err
		}
	}

	return nil
}

func listOldVolumes(unit string) ([]database.LocalVolume, map[string][]database.LUN, []database.LUN, error) {
	// local volumes
	lvs, err := database.ListVolumesByUnitID(unit)
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
func migrateVolumes(storage storage.Store, nodeID string,
	engine *cluster.Engine,
	localStore, oldLVs []database.LocalVolume,
	lunMap map[string][]database.LUN,
	lunSlice []database.LUN) ([]database.LocalVolume, error) {

	// Mapping LVs
	if len(lunSlice) > 0 && storage == nil {
		return nil, errors.New("Store is required")
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
	//	for vg, list := range vgMap {
	//		l, size := make([]int, len(list)), 0

	//		for i := range list {
	//			l[i] = list[i].HostLunID
	//			size += list[i].SizeByte
	//		}

	//		config := sdk.VgConfig{
	//			HostLunID: l,
	//			VgName:    vg,
	//			Type:      storage.Vendor(),
	//		}

	//		err := sdk.SanVgCreate(addr, config)
	//		if err != nil {
	//			return nil, err
	//		}
	//	}

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

	// remove old container
	err := engine.RemoveContainer(old, true, true)
	engine.CheckConnectionErr(err)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Engine":    engine.Addr,
			"Container": old.Info.Name,
		}).WithError(err).Error("remove container")
	}

	// remove old LocalVolume
	for i := range lvs {
		err := engine.RemoveVolume(lvs[i].Name)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Engine":    engine.Addr,
				"Container": old.Info.Name,
				"Volume":    lvs[i].Name,
			}).WithError(err).Error("remove Volume")

			return err
		}

		logrus.WithFields(logrus.Fields{
			"Engine":    engine.Addr,
			"Container": old.Info.Name,
			"Volume":    lvs[i].Name,
		}).Info("remove Volume")
	}

	return nil
}

func updateUnit(unit database.Unit, lvs []database.LocalVolume, reserveSAN bool) error {
	return database.TxUpdateMigrateUnit(unit, lvs, reserveSAN)
}

// UnitRebuild rebuild the unit in another host
func (gd *Gardener) UnitRebuild(nameOrID string, candidates []string, hostConfig *ctypes.HostConfig) (string, error) {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return "", err
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return "", err
	}

	svc.RLock()

	rebuild, err := svc.getUnit(table.ID)
	if err != nil {
		svc.RUnlock()

		svc, err = gd.reloadService(table.ServiceID)
		if err != nil {
			return "", err
		}

		svc.RLock()

		rebuild, err = svc.getUnit(table.ID)
		if err != nil {
			svc.RUnlock()

			return "", err
		}
	}

	entry := logrus.WithFields(logrus.Fields{
		"Service": svc.Name,
		"Rebuild": rebuild.Name,
	})

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

	dc, original, err := gd.getNode(rebuild.EngineID)
	if err != nil || dc == nil {
		return "", err
	}

	out, err := dc.listCandidates(candidates)
	if err != nil {
		return "", err
	}
	entry.Debugf("list Candidates:%d", len(out))

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
				entry.Errorf("Unit rebuild failed,%+v", err)
				// error handle
				_err := gd.resourceRecycle([]*pendingAllocResource{pending})
				if _err != nil {
					entry.Errorf("Recycle,%+v", _err)
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
				_err := sanDeactivateAndDelMapping(dc.store, original.engine.IP, lunMap, lunSlice)
				if _err != nil {
					entry.Errorf("san Deactivate and delete Mapping,%+v", _err)
				}
			}

			entry.Debug("recycle old container volumes resource")
			// recycle lun
			for i := range lunSlice {
				_err := dc.store.Recycle(lunSlice[i].ID, 0)
				if _err != nil {
					entry.Errorf("Store recycle,%+v", _err)
				}
			}

			// clean local volumes
			for i := range oldLVs {
				if isSanVG(oldLVs[i].VGName) {
					continue
				}
				_err := original.localStore.Recycle(oldLVs[i].ID)
				if err != nil {
					entry.Errorf("local Store recycle,%+v", _err)
				}
			}

			// clean database
			_err := database.TxDeleteVolumes(oldLVs)
			if _err != nil {
				entry.Errorf("%+v", _err)
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
			return err
		}

		if svc.authConfig == nil {
			svc.authConfig, err = gd.registryAuthConfig()
			if err != nil {
				return err
			}
		}

		entry.WithField("Engine", engine.Addr).Debug("create container")

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
			entry.Debugf("clean created container %s", c.ID)

			_err := cleanOldContainer(c, lvs)
			if _err != nil {
				entry.Errorf("clean old container,%+v", _err)
			}
			_err = removeNetworkings(addr, networkings)
			if _err != nil {
				entry.Errorf("remove Networkings error:%+v", _err)
			}

		}(container, engine.IP, networkings, pending.localStore)

		if rebuild.Type != _SwitchManagerType {
			err := svc.isolate(rebuild.Name)
			if err != nil {
				entry.Errorf("isolate container:%+v", err)
			}
		}

		err = stopOldContainer(rebuild)
		if err != nil {
			return err
		}

		err = startUnit(engine, container.ID, rebuild, networkings, pending.localStore)
		if err != nil {
			return err
		}

		err = engine.RenameContainer(container, rebuild.Name)
		if err != nil {
			return errors.Wrap(err, "rename container")
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
			return err
		}

		err = saveContainerToConsul(container)
		if err != nil {
			entry.Errorf("save container to Consul:%+v", err)
			// return err
		}

		oldEngineIP := oldContainer.Engine.IP
		err = cleanOldContainer(oldContainer, oldLVs)
		if err != nil {
			entry.Errorf("clean old container,%+v", err)
		}

		sys, err := gd.systemConfig()
		if err != nil {
			entry.WithError(err).Error("get System Config")
		}

		err = deregisterToServices(oldEngineIP, rebuild.ID)
		if err != nil {
			entry.Errorf("deregister service,%+v", err)
		}

		err = registerToServers(rebuild, svc, sys)
		if err != nil {
			entry.Errorf("register service:%+v", err)
		}

		if rebuild.Type != _SwitchManagerType {
			// switchback unit
			err = svc.switchBack(rebuild.Name)
			if err != nil {
				entry.Errorf("switchBack error:%+v", err)
			}
		}

		return nil
	}

	task := database.NewTask(rebuild.Name, unitRebuildTask, rebuild.ID, "", nil, 0)

	create := func() error {
		rebuild.Status, rebuild.LatestError = statusUnitRebuilding, ""

		return database.TxUpdateUnitAndInsertTask(&rebuild.Unit, task)
	}

	update := func(code int, msg string) error {
		task.Status = int64(code)

		return database.TxUpdateUnitStatusWithTask(&rebuild.Unit, &task, msg)
	}

	t := NewAsyncTask(context.Background(),
		background,
		create,
		update,
		0)

	return task.ID, t.Run()
}
