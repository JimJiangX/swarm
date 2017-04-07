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
	"github.com/docker/swarm/cluster/swarm/storage"
	"github.com/docker/swarm/scheduler/node"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
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
		for i := range nodes {
			if nodes[i].Status != statusNodeEnable {
				continue
			}

			out = append(out, nodes[i])
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
			return nil, errors.Wrap(err, "get CPU num")
		}
		clone.HostConfig.CpusetCpus = strconv.FormatInt(ncpu, 10)
	}

	clone.HostConfig.Binds = make([]string, 0, 5)

	return clone, nil
}

// UnitMigrate migrate the assigned unit to another host
func (gd *Gardener) UnitMigrate(nameOrID string, candidates []string, hostConfig *ctypes.HostConfig, force bool) (_ string, err error) {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return "", err
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return "", err
	}

	done, val, err := svc.statusLock.CAS(statusServiceUnitMigrating, isStatusNotInProgress)
	if err != nil {
		return "", err
	}
	if !done {
		return "", errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	entry := logrus.WithField("Service", svc.Name)

	svc.RLock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		if err != nil {
			_err := svc.statusLock.SetStatus(statusServiceUnitMigrateFailed)
			if _err != nil {
				entry.Errorf("%+v", _err)
			}
		}
	}()

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

	entry = entry.WithField("Migrate", migrate.Name)

	users, err := svc.getUsers()
	if err != nil {
		return "", err
	}

	if migrate.container == nil {
		migrate.container, err = getContainerFromConsul(migrate.ContainerID)
		if err != nil {
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

	dc, original, err := gd.getNode(migrate.EngineID)
	if err != nil || dc == nil || (san && dc.store == nil) {
		if !force || dc == nil {
			return "", errors.Errorf("getNode error:%s,dc=%p,dc.store==nil:%t", err, dc, dc.store == nil)
		}
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

	swarmID := gd.generateUniqueID()
	gd.scheduler.Lock()

	engine, err := gd.selectEngine(config, module, out, filters)
	if err != nil {
		gd.scheduler.Unlock()

		return "", err
	}

	config.SetSwarmID(swarmID)
	gd.pendingContainers[swarmID] = &pendingContainer{
		Name:   swarmID,
		Config: config,
		Engine: engine,
	}

	gd.scheduler.Unlock()

	background := func(ctx context.Context) (err error) {
		pending := newPendingAllocResource()

		svc.Lock()

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%v", r)
			}
			svcStatus := statusServiceUnitMigrated
			if err == nil {
				entry.Infof("migrate service done,%v", err)
				migrate.Status, migrate.LatestError = statusUnitMigrated, ""
			} else {
				gd.scheduler.Lock()
				delete(gd.pendingContainers, swarmID)
				gd.scheduler.Unlock()

				entry.Errorf("%+v", err)
				migrate.Status, migrate.LatestError, svcStatus = statusUnitMigrateFailed, err.Error(), statusServiceUnitMigrateFailed

				entry.Errorf("len(localVolume)=%d", len(pending.localStore))
				// clean local volumes
				for _, lv := range pending.localStore {
					if isSanVG(lv.VGName) {
						continue
					}

					entry.Debugf("recycle localVolume,VG=%s,Name=%s", lv.VGName, lv.Name)

					_err := database.DeleteLocalVoume(lv.Name)
					if _err != nil {
						entry.Errorf("delete localVolume %s,%+v", lv.Name, _err)
					}
				}
			}

			_err := svc.statusLock.SetStatus(svcStatus)
			if _err != nil {
				entry.Errorf("defer update service status:%+v", _err)
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
			}
		}

		if !force {
			err = stopOldContainer(migrate)
			if err != nil {
				return err
			}
		}

		oldLVs, lunMap, lunSlice, err := listOldVolumes(migrate.ID)
		if err != nil {
			return err
		}

		if len(lunMap) > 0 && dc.store != nil {
			if !force {
				err = sanDeactivate(dc.store.Vendor(), original.engine.IP, lunMap)
				if err != nil {
					time.Sleep(3 * time.Second)

					_err := sanActivate(original.engine.IP, lunMap)
					if _err != nil {
						return errors.Errorf("sanDeactivate error:%+v\nThen exec sanActivate error:%+v", err, _err)
					}

					return err
				}
			}

			for i := range lunSlice {
				err = dc.store.DelMapping(lunSlice[i].ID)
				if err != nil {
					logrus.Errorf("%s DelMapping %s", dc.store.Vendor(), lunSlice[i].ID)
					return err
				}
			}

			if !force {
				err = removeSCSI(dc.store.Vendor(), original.engine.IP, lunMap)
				if err != nil {
					return err
				}
			}
		}

		defer func() {
			if force {
				return
			}
			if err != nil {
				entry.Errorf("%+v", err)

				if !force && len(lunMap) > 0 && dc.store != nil {
					_err := sanDeactivate(dc.store.Vendor(), engine.IP, lunMap)
					if _err != nil {
						entry.Errorf("defer san Deactivate And DelMapping,%+v", _err)
					}

					for i := range lunSlice {
						_err := dc.store.DelMapping(lunSlice[i].ID)
						if _err != nil {
							logrus.Errorf("%s DelMapping %s,%+v", dc.store.Vendor(), lunSlice[i].ID, _err)
						}
					}

					_err = migrateVolumes(dc.store, original.ID, original.engine, oldLVs, lunMap, lunSlice)
					if _err != nil {
						entry.Errorf("defer migrate volumes,%+v", _err)
						//	return err
					}
				}

				return
			}

			// clean local volumes
			for i := range oldLVs {
				if isSanVG(oldLVs[i].VGName) {
					continue
				}

				entry.Debugf("recycle old localVolume,VG=%s,Name=%s", oldLVs[i].VGName, oldLVs[i].Name)

				_err := original.localStore.Recycle(oldLVs[i].ID)
				if _err != nil {
					entry.Errorf("defer recycle local volume,%+v", _err)
				}
			}
		}()

		pending.unit = migrate
		pending.engine = engine

		err = gd.allocStorage(pending, engine, config, module.Stores, true)
		if err != nil {
			return err
		}

		dc, node, err := gd.getNode(engine.ID)
		if err != nil {
			return err
		}

		lvs := make([]database.LocalVolume, len(pending.localStore), len(oldLVs))
		copy(lvs, pending.localStore)
		for i := range oldLVs {
			if isSanVG(oldLVs[i].VGName) {
				lvs = append(lvs, oldLVs[i])
			}
		}

		err = migrateVolumes(dc.store, node.ID, engine, lvs, lunMap, lunSlice)
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

		gd.scheduler.Lock()
		delete(gd.pendingContainers, swarmID)
		gd.scheduler.Unlock()

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

		err = startUnit(engine, container.ID, migrate, users, networkings, lvs, false)

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

		err = updateUnit(migrate.Unit, oldLVs, true)
		if err != nil {
			return err
		}

		if migrate.Type == _SwitchManagerType {
			if err = swmInitTopology(svc, migrate, users); err != nil {
				entry.Errorf("init Topology:%+v", err)
			}
		}

		err = saveContainerToConsul(container)
		if err != nil {
			logrus.Errorf("Save Container To Consul error:%+v", err)
			// return err
		}

		oldEngineIP := oldContainer.Engine.IP
		if !force {
			err = cleanOldContainer(oldContainer, oldLVs)
			if err != nil {
				logrus.Error(err)
			}
		}

		sys, err := gd.systemConfig()
		if err != nil {
			logrus.WithError(err).Error("get System Config")
		}

		err = deregisterToServices(oldEngineIP, migrate.ID)
		err = registerToServers(migrate, svc, sys)

		//		if migrate.Type != _SwitchManagerType {
		//			// switchback unit
		//			err = svc.switchBack(migrate.Name)
		//			if err != nil {
		//				entry.Errorf("switchBack error:%+v", err)
		//			}
		//		}

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

func startUnit(engine *cluster.Engine, containerID string, u *unit, users []database.User,
	networkings []IPInfo, lvs []database.LocalVolume, init bool) error {

	err := startContainer(containerID, engine, networkings)
	if err != nil {
		return err
	}

	var (
		cnf = 0
		cmd []string
	)

	for i := range lvs {
		if strings.Contains(lvs[i].Name, "_CNF_LV") {
			cnf = i
			break
		}
		if strings.Contains(lvs[i].Name, "_DAT_LV") {
			cnf = i
		}
	}

	if !init && isSanVG(lvs[cnf].VGName) {
		cmd = u.StartServiceCmd()
	} else {

		args := make([]string, 0, len(users)*3)
		for i := range users {
			if users[i].Type != User_Type_DB {
				continue
			}

			args = append(args, users[i].Role, users[i].Username, users[i].Password)
		}

		cmd = u.InitServiceCmd(args...)

		logrus.Debug("copy Service Config")
		// TODO:update config content after update image
		err = copyConfigIntoCNFVolume(engine.IP, u.Path(), u.parent.Content, lvs)
		if err != nil {
			return err
		}
	}

	logrus.Debug("Start Service")

	if len(cmd) == 0 {
		logrus.WithField("Container", containerID).Warn("cmd is nil")
		return nil
	}

	_, err = containerExec(context.Background(), engine, containerID, cmd, false)

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
		if _err.Error() != "EOF" && _err != errContainerNotFound && _err != errContainerNotRunning {
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

func sanDeactivate(vendor, host string, lunMap map[string][]database.LUN) error {
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
			Vendor:    vendor,
		}
		// san volumes
		addr := getPluginAddr(host, pluginPort)
		err := sdk.SanDeActivate(addr, config)
		if err != nil {
			logrus.Errorf("%s SanDeActivate error:%s", host, err)
			return err
		}
	}

	return nil
}

func removeSCSI(vendor, host string, lunMap map[string][]database.LUN) error {
	for _, list := range lunMap {
		hostLuns := make([]int, len(list))
		for i := range list {
			hostLuns[i] = list[i].HostLunID
		}

		addr := getPluginAddr(host, pluginPort)

		opt := sdk.RemoveSCSIConfig{
			Vendor:    vendor,
			HostLunId: hostLuns,
		}

		err := sdk.RemoveSCSI(addr, opt)
		if err != nil {
			logrus.Errorf("%s RemoveSCSI error:%s", host, err)
			return err
		}
	}

	return nil
}

func sanActivate(host string, lunMap map[string][]database.LUN) error {
	for vg, list := range lunMap {
		names := make([]string, len(list))
		for i := range list {
			names[i] = list[i].Name
		}
		config := sdk.ActiveConfig{
			VgName: vg,
			Lvname: names,
		}
		// san volumes
		addr := getPluginAddr(host, pluginPort)
		err := sdk.SanActivate(addr, config)
		if err != nil {
			logrus.Errorf("%s SanActivate error:%s", host, err)
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
	lvs []database.LocalVolume,
	lunMap map[string][]database.LUN,
	lunSlice []database.LUN) error {

	// Mapping LVs
	if len(lunSlice) > 0 && storage == nil {
		return errors.New("Store is required")
	}

	for i := range lunSlice {
		err := storage.Mapping(nodeID, lunSlice[i].VGName, lunSlice[i].ID)
		if err != nil {
			return err
		}
	}

	vgMap := make(map[string][]database.LUN, len(lunMap))
	for vg := range lunMap {
		if isSanVG(vg) {
			out, err := database.ListLUNByVgName(vg)
			if err != nil {
				return err
			}
			if len(out) > 0 {
				vgMap[vg] = out
			}
		}
	}

	err := sanActivate(engine.IP, vgMap)
	if err != nil {
		return err
	}

	for i := range lvs {
		if isSanVG(lvs[i].VGName) {
			continue
		}

		_, err := createVolume(engine, lvs[i])
		if err != nil {
			return err
		}
	}

	return nil
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
		if isSanVG(lvs[i].VGName) {
			continue
		}
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
func (gd *Gardener) UnitRebuild(nameOrID, image string, candidates []string, hostConfig *ctypes.HostConfig) (string, error) {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return "", err
	}

	var im Image
	if image != "" {

		parts := strings.SplitN(image, ":", 2)
		if len(parts) == 2 {
			image, im, err = gd.getImageName("", parts[0], parts[1])
		} else {
			image, im, err = gd.getImageName(image, "", "")
		}
		if err != nil {
			return "", err
		}
	}

	svc, err := gd.GetService(table.ServiceID)
	if err != nil {
		return "", err
	}

	done, val, err := svc.statusLock.CAS(statusServiceUnitRebuilding, isStatusNotInProgress)
	if err != nil {
		return "", err
	}
	if !done {
		return "", errors.Errorf("Service %s status conflict,got (%x)", svc.Name, val)
	}

	entry := logrus.WithField("Service", svc.Name)

	svc.RLock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		if err != nil {
			_err := svc.statusLock.SetStatus(statusServiceUnitRebuildFailed)
			if _err != nil {
				entry.Errorf("%+v", _err)
			}
		}
	}()

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

	entry = entry.WithField("Rebuild", rebuild.Name)

	users, err := svc.getUsers()
	if err != nil {
		return "", err
	}

	if rebuild.container == nil {
		rebuild.container, err = getContainerFromConsul(rebuild.ContainerID)
		if err != nil {
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

	if im.ImageID != "" {

		rebuild.Unit.ImageID = im.ImageID
		rebuild.Unit.ImageName = im.Name + ":" + im.Version

		config.Config.Image = image
		config.Config.Labels[_ImageIDInRegistryLabelKey] = im.ImageID
	}

	swarmID := gd.generateUniqueID()
	gd.scheduler.Lock()

	engine, err := gd.selectEngine(config, module, out, filters)
	if err != nil {
		gd.scheduler.Unlock()

		return "", err
	}

	config.SetSwarmID(swarmID)
	gd.pendingContainers[swarmID] = &pendingContainer{
		Name:   swarmID,
		Config: config,
		Engine: engine,
	}

	gd.scheduler.Unlock()

	background := func(ctx context.Context) (err error) {
		pending := newPendingAllocResource()

		svc.Lock()

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%v", r)
			}
			svcStatus := statusServiceUnitRebuilt
			if err == nil {
				rebuild.Status, rebuild.LatestError = statusUnitRebuilt, ""
			} else {
				gd.scheduler.Lock()
				delete(gd.pendingContainers, swarmID)
				gd.scheduler.Unlock()

				rebuild.Status, rebuild.LatestError, svcStatus = statusUnitRebuildFailed, err.Error(), statusServiceRestoreFailed

				entry.Errorf("len(localVolume)=%d", len(pending.localStore))
				// clean local volumes
				for _, lv := range pending.localStore {
					if isSanVG(lv.VGName) {
						continue
					}

					entry.Debugf("recycle localVolume,VG=%s,Name=%s", lv.VGName, lv.Name)

					_err := database.DeleteLocalVoume(lv.Name)
					if _err != nil {
						entry.Errorf("delete localVolume %s,%+v", lv.Name, _err)
					}
				}
			}

			_err := svc.statusLock.SetStatus(svcStatus)
			if _err != nil {
				entry.Errorf("%+v", _err)
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

		oldLVs, lunMap, _, err := listOldVolumes(rebuild.ID)
		if err != nil {
			return err
		}

		defer func() {
			if err != nil {
				return
			} else {
				entry.Errorf("%+v", err)
			}
			// deactivate
			// del mapping
			if len(lunMap) > 0 && dc.store != nil {

				for vg, list := range lunMap {
					_err := removeVGAndLUN(original.engine.IP, vg, list)
					if _err != nil {
						entry.Errorf("san remove VG and LUN,%+v", _err)
					}
				}
			}

			// clean local volumes
			for i := range oldLVs {
				if isSanVG(oldLVs[i].VGName) {
					continue
				}
				_err := original.localStore.Recycle(oldLVs[i].ID)
				if _err != nil {
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

		gd.scheduler.Lock()
		delete(gd.pendingContainers, swarmID)
		gd.scheduler.Unlock()

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

		err = startUnit(engine, container.ID, rebuild, users, networkings, pending.localStore, true)
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

		err = updateUnit(rebuild.Unit, oldLVs, false)
		if err != nil {
			return err
		}

		if rebuild.Type == _SwitchManagerType {
			if err = swmInitTopology(svc, rebuild, users); err != nil {
				entry.Errorf("init Topology:%+v", err)
			}
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

		//		if rebuild.Type != _SwitchManagerType {
		//			// switchback unit
		//			err = svc.switchBack(rebuild.Name)
		//			if err != nil {
		//				entry.Errorf("switchBack error:%+v", err)
		//			}
		//		}

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
