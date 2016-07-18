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

	svc.RLock()

	index, module := 0, structs.Module{}
	filters := make([]string, len(svc.units))
	for i, u := range svc.units {
		filters[i] = u.EngineID
		if u.ID == table.ID {
			index = i
		}
	}
	u := svc.units[index]
	oldContainer := u.container

	san := false
	for i := range svc.base.Modules {
		if u.Type == svc.base.Modules[i].Type {
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

	if !san {
		return "", errors.Errorf("Unit %s storage hasn't SAN Storage,Cannot Exec Migrate", nameOrID)
	}

	dc, err := gd.DatacenterByEngine(u.EngineID)
	if err != nil || dc == nil {
		return "", err
	}

	out, err := dc.listCandidates(candidates)
	if err != nil {
		return "", err
	}
	logrus.Debugf("listCandidates:%d", out)

	config, err := resetContainerConfig(u.container.Config, hostConfig)
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

		gd.scheduler.Lock()
		defer func() {
			gd.scheduler.Unlock()

			if err != nil {
				// error handle
				logrus.Error(err)
			}
		}()

		cpuset, err := gd.allocCPUs(engine, config.HostConfig.CpusetCpus)
		if err != nil {
			logrus.Errorf("Alloc CPU '%s' Error:%s", config.HostConfig.CpusetCpus, err)
			return err
		}
		config.HostConfig.CpusetCpus = cpuset

		svc.Lock()
		defer svc.Unlock()

		err = stopOldContainer(svc, u)
		if err != nil {
			return err
		}

		oldLVs, lunMap, lunSlice, err := listOldVolumes(u.ID)
		if err != nil {
			return err
		}

		if len(lunMap) > 0 {
			err = sanDeactivateAndDelMapping(dc.storage, u, lunMap, lunSlice)
			if err != nil {
				return err
			}
		}

		pending.unit = u
		pending.engine = engine

		err = gd.allocStorage(pending, engine, config, module.Stores, true)
		if err != nil {
			return err
		}

		swarmID := gd.generateUniqueID()
		config.SetSwarmID(swarmID)
		gd.pendingContainers[swarmID] = &pendingContainer{
			Name:   u.Name,
			Config: config,
			Engine: engine,
		}

		logrus.Debugf("[MG]start pull image %s", config.Image)
		authConfig, err := gd.RegistryAuthConfig()
		if err != nil {
			return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
		}

		err = pullImage(engine, config.Image, authConfig)
		if err != nil {
			return fmt.Errorf("pullImage Error:%s", err)
		}

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

		container, err := engine.Create(config, u.Name, false, authConfig)
		if err != nil {
			return err
		}

		err = startUnit(engine, container.ID, u, lvs)
		if err != nil {
			return err
		}
		delete(gd.pendingContainers, swarmID)

		sys, err := database.GetSystemConfig()
		if err != nil {
			return err
		}
		err = cleanOldContainer(u.ID, oldContainer, oldLVs, *sys)
		if err != nil {
			return err
		}

		container, err = container.Refresh()
		if err != nil {
			logrus.Warnf("containe Refresh Erorr:%s", err)
		}

		err = gd.SaveContainerToConsul(container)
		if err != nil {
			logrus.Errorf("Save Container To Consul error:%s", err)
			// return err
		}

		u.container = container
		u.ContainerID = container.ID
		u.engine = engine
		u.EngineID = engine.ID
		u.CreatedAt = time.Now()

		err = updateUnit(u.Unit, oldLVs, false)
		if err != nil {
			logrus.Errorf("updateUnit in database error:%s", err)

			return err
		}

		err = registerToServers(u, svc, *sys)
		if err != nil {
			logrus.Errorf("registerToServers error:%s", err)
		}
		// switchback unit
		err = svc.switchBack(u.Name)
		if err != nil {
			logrus.Errorf("switchBack error:%s", err)
		}

		return nil
	}

	task := database.NewTask(_Unit_Migrate_Task, u.ID, "", nil, 0)

	t := NewAsyncTask(context.Background(), background, task.Insert, task.UpdateStatus, 0)

	return task.ID, t.Run()
}

func startUnit(engine *cluster.Engine, containerID string,
	u *unit, lvs []database.LocalVolume) error {
	logrus.Debug("starting Containers")

	err := startContainer(containerID, engine, u.networkings)
	if err != nil {
		return err
	}

	logrus.Debug("copy Service Config")
	err = copyConfigIntoCNFVolume(engine, lvs, u.Path(), u.parent.Content)
	if err != nil {
		return err
	}

	logrus.Debug("init & Start Service")
	err = initUnitService(containerID, engine, u.InitServiceCmd())
	if err != nil {
		logrus.Errorf("")
	}
	return err
}

func stopOldContainer(svc *Service, u *unit) error {
	if u.Type != _SwitchManagerType {
		err := svc.isolate(u.Name)
		if err != nil {
			logrus.Errorf("isolate container %s error:%s", u.Name, err)
		}
	}
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

func sanDeactivateAndDelMapping(storage store.Store, u *unit,
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
		err := u.deactivateVG(config)
		if err != nil {
			return err
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
			list, err := database.ListLUNByVgName(vg)
			if err != nil {
				return nil, nil, nil, err
			}
			if len(list) > 0 {
				lunMap[vg] = list
				lunSlice = append(lunSlice, list...)
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

	addr := getPluginAddr(engine.IP, pluginPort)

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
			Type:      storage.Vendor(),
		}

		err := sdk.SanVgCreate(addr, config)
		if err != nil {
			return nil, err
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
		err := sdk.SanActivate(addr, activeConfig)
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

func cleanOldContainer(unitID string, old *cluster.Container, lvs []database.LocalVolume, sys database.Configurations) error {
	engine := old.Engine
	if engine == nil {
		return errEngineIsNil
	}

	// remove old container
	err := engine.RemoveContainer(old, true, true)
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
	}

	configs := sys.GetConsulConfigs()
	if len(configs) == 0 {
		return fmt.Errorf("GetConsulConfigs error %v %v", err, configs[0])
	}
	err = deregisterHealthCheck(engine.IP, unitID, configs[0])
	if err != nil {
		return err
	}

	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)

	err = deregisterToHorus(horus, []deregisterService{{unitID}})
	if err != nil {
		logrus.Errorf("deregisterToHorus error:%s,endpointer:%s", err, unitID)
	}

	return err
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

	svc.RLock()

	index, module := 0, structs.Module{}
	filters := make([]string, len(svc.units))
	for i, u := range svc.units {
		filters[i] = u.EngineID
		if u.ID == table.ID {
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

	dc, original, err := gd.GetNode(u.EngineID)
	if err != nil || dc == nil {
		return "", err
	}

	out, err := dc.listCandidates(candidates)
	if err != nil {
		return "", err
	}
	logrus.Debugf("listCandidates:%d", len(out))

	config, err := resetContainerConfig(u.container.Config, hostConfig)
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

		sys, err := database.GetSystemConfig()
		if err != nil {
			return err
		}

		cpuset, err := gd.allocCPUs(engine, config.HostConfig.CpusetCpus)
		if err != nil {
			logrus.Errorf("Alloc CPU '%s' Error:%s", config.HostConfig.CpusetCpus, err)
			return err
		}
		config.HostConfig.CpusetCpus = cpuset

		oldLVs, lunMap, lunSlice, err := listOldVolumes(u.ID)
		if err != nil {
			return err
		}

		defer func() {
			if err != nil {
				return
			}
			err = stopOldContainer(svc, u)
			if err != nil {
				logrus.Error(err)
			}

			// deactivate
			// del mapping
			if len(lunMap) > 0 {
				err = sanDeactivateAndDelMapping(dc.storage, u, lunMap, lunSlice)
				if err != nil {
					logrus.Error(err)
				}
			}
			// recycle lun
			for i := range lunSlice {
				err := dc.storage.Recycle(lunSlice[i].ID, 0)
				if err != nil {
					logrus.Error(err)
				}
			}

			// clean local volumes
			for i := range oldLVs {
				err := original.localStore.Recycle(oldLVs[i].ID)
				if err != nil {
					logrus.Error(err)
				}
			}

			err = cleanOldContainer(u.ID, oldContainer, oldLVs, *sys)
			if err != nil {
				logrus.Error(err)
			}

			// clean database
		}()

		pending.unit = u
		pending.engine = engine

		err = gd.allocStorage(pending, engine, config, module.Stores, false)
		if err != nil {
			return err
		}

		swarmID := gd.generateUniqueID()
		config.SetSwarmID(swarmID)
		gd.pendingContainers[swarmID] = &pendingContainer{
			Name:   u.Name,
			Config: config,
			Engine: engine,
		}

		logrus.Debugf("[MG]start pull image %s", config.Image)
		authConfig, err := gd.RegistryAuthConfig()
		if err != nil {
			return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
		}

		err = pullImage(engine, config.Image, authConfig)
		if err != nil {
			return fmt.Errorf("pullImage Error:%s", err)
		}

		err = createServiceResources(gd, []*pendingAllocResource{pending})
		if err != nil {
			return err
		}

		container, err := engine.Create(config, u.Name, false, authConfig)
		if err != nil {
			return err
		}
		delete(gd.pendingContainers, swarmID)

		err = startUnit(engine, container.ID, u, pending.localStore)
		if err != nil {
			return err
		}

		container, err = container.Refresh()
		if err != nil {
			logrus.Warnf("containe Refresh Erorr:%s", err)
		}

		err = gd.SaveContainerToConsul(container)
		if err != nil {
			logrus.Errorf("Save Container To Consul error:%s", err)
			// return err
		}

		u.container = container
		u.ContainerID = container.ID
		u.engine = engine
		u.EngineID = engine.ID
		u.CreatedAt = time.Now()

		err = updateUnit(u.Unit, oldLVs, true)
		if err != nil {
			logrus.Errorf("updateUnit in database error:%s", err)
			return err
		}

		err = registerToServers(u, svc, *sys)
		if err != nil {
			logrus.Errorf("registerToServers error:%s", err)
		}
		return err
	}

	task := database.NewTask(_Unit_Rebuild_Task, u.ID, "", nil, 0)

	t := NewAsyncTask(context.Background(), background, task.Insert, task.UpdateStatus, 0)

	return task.ID, t.Run()
}

func registerToServers(u *unit, svc *Service, sys database.Configurations) error {
	logrus.Debug("[MG]registerServices")
	if err := registerHealthCheck(u, sys.ConsulConfig, svc); err != nil {
		return err
	}

	logrus.Debug("[MG]registerToHorus")
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
