package swarm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/storage"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
)

func (gd *Gardener) allocNetworking(pending *pendingAllocResource, config *cluster.ContainerConfig) error {
	u := pending.unit
	req := u.Requirement()

	ports, portENV, err := allocPorts(req.ports, u.ID, u.Name)
	if err != nil {
		return err
	}
	if portENV != "" {
		config.Env = append(config.Env, portENV)
	}
	u.ports = ports

	networkings, err := gd.allocNetworkings(u.ID, pending.engine, req.networkings, config)
	if err != nil {
		return err
	}
	u.networkings = networkings

	return nil
}

// unit == unit.ID
func (gd *Gardener) allocNetworkings(unit string, engine *cluster.Engine,
	req []netRequire, config *cluster.ContainerConfig) ([]IPInfo, error) {

	networkings, err := gd.getNetworkingSetting(engine, unit, req)
	if err != nil {
		return networkings, err
	}

	for i := range networkings {
		if networkings[i].Type == _ContainersNetworking {
			ip := networkings[i].IP.String()
			config.Env = append(config.Env, fmt.Sprintf("IPADDR=%s", ip))
			config.Labels[_ContainerIPLabelKey] = ip
			config.Labels[_NetworkingLabelKey] = networkings[i].String()
		} else if networkings[i].Type == _ExternalAccessNetworking {
			config.Labels[_ProxyNetworkingLabelKey] = networkings[i].String()
		}
	}

	return networkings, nil
}

func allocPorts(need []port, unitID, unitName string) ([]database.Port, string, error) {
	length := len(need)

	if length == 0 {
		logrus.Warning("no need ports")
		return nil, "", nil
	}

	ports, err := database.ListAvailablePorts(length)
	if err != nil || len(ports) < length {
		logrus.Errorf("Alloc Ports Error:%v", err)

		return nil, "", errors.Errorf("no enough available ports(%d<%d),%+v", len(ports), length, err)
	}

	for i := range need {
		ports[i].Name = need[i].name
		ports[i].Proto = need[i].proto
		ports[i].UnitID = unitID
		ports[i].UnitName = unitName
		ports[i].Allocated = true
	}

	err = database.TxUpdatePortSlice(ports)
	if err != nil {
		return nil, "", err
	}

	portSlice := make([]string, len(ports))
	for i := range ports {
		portSlice[i] = strconv.Itoa(ports[i].Port)
	}

	env := fmt.Sprintf("PORT=%s", strings.Join(portSlice, ","))

	return ports, env, nil
}

func (gd *Gardener) allocCPUs(engine *cluster.Engine, cpusetCpus string, reserve ...string) (string, error) {
	ncpu, err := parseCpuset(cpusetCpus)
	if err != nil {
		return "", err
	}

	total := int(engine.Cpus)
	used := int(engine.UsedCpus())

	if total-used < ncpu {
		return "", errors.Errorf("alloc CPU,%s CPU is short(%d-%d<%d)", engine.Name, total, used, ncpu)
	}

	list := make([]string, len(reserve), len(reserve)+len(gd.pendingContainers))
	copy(list, reserve)

	for _, pending := range gd.pendingContainers {
		if pending.Engine.ID == engine.ID {
			list = append(list, pending.Config.HostConfig.CpusetCpus)
		}
	}

	usedCPUs := parseUintList(list)

	if total-used-len(usedCPUs) < ncpu {
		return "", errors.Errorf("alloc CPU error,%s CPU is Short(%d-%d-%d<%d),", engine.Name, total, used, len(usedCPUs), ncpu)
	}

	free := make([]string, ncpu)
	for i, n := 0, 0; i < total && n < ncpu; i++ {
		if !usedCPUs[i] {
			free[n] = strconv.Itoa(i)
			n++
		}
	}

	return strings.Join(free, ","), nil
}

func parseUintList(list []string) map[int]bool {
	if len(list) == 0 {
		return map[int]bool{}
	}

	ints := make(map[int]bool, len(list)*3)

	for i := range list {
		cpus, err := utils.ParseUintList(list[i])
		if err != nil {
			logrus.Errorf("parseUintList '%s' error:%s", list[i], err)
			continue
		}
		for k, v := range cpus {
			if v {
				ints[k] = v
			}
		}
	}

	return ints
}

type pendingAllocResource struct {
	unit   *unit
	engine *cluster.Engine
	// pendingContainer *pendingContainer
	swarmID string
	// ports       []database.Port
	// networkings []IPInfo
	localStore []database.LocalVolume
	sanStore   []database.LUN
}

func newPendingAllocResource() *pendingAllocResource {
	return &pendingAllocResource{
		localStore: make([]database.LocalVolume, 0, 5),
		sanStore:   make([]database.LUN, 0, 2),
	}
}

func createVolumes(engine *cluster.Engine, lvs []database.LocalVolume) ([]*types.Volume, error) {
	logrus.Debugf("Engine %s create volumes %d", engine.Addr, len(lvs))
	volumes := make([]*types.Volume, 0, len(lvs))
	vglist := make(map[string]struct{}, len(lvs))

	for i := range lvs {
		vglist[lvs[i].VGName] = struct{}{}
	}
	for vg := range vglist {
		// if volume create on san storage,should created VG before create Volume
		if isSanVG(vg) {
			err := createSanStoreageVG(engine.IP, vg)
			if err != nil {
				return volumes, err
			}
		}
	}
	for i := range lvs {
		volume, err := createVolume(engine, lvs[i])
		if err != nil {
			return volumes, err
		}

		volumes = append(volumes, volume)
	}

	return volumes, nil
}

func (gd *Gardener) resourceRecycle(pendings []*pendingAllocResource) (err error) {
	gd.scheduler.Lock()
	for i := range pendings {

		if pendings[i] == nil || pendings[i].swarmID == "" {
			continue
		}
		delete(gd.pendingContainers, pendings[i].swarmID)
	}
	gd.scheduler.Unlock()

	tx, err := database.GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	gd.Lock()
	defer gd.Unlock()

	for i := range pendings {
		if pendings[i] == nil {
			continue
		}

		if pendings[i].unit != nil {
			database.TxDeleteUnit(tx, pendings[i].unit.Unit.ServiceID)
			database.TxDeleteVolume(tx, pendings[i].unit.Unit.ID)
		}

		for _, lv := range pendings[i].localStore {
			database.TxDeleteVolume(tx, lv.ID)
		}
	}

	err = tx.Commit()

	return errors.Wrap(err, "alloction resource recycle")
}

func (gd *Gardener) allocStorage(
	penging *pendingAllocResource,
	engine *cluster.Engine,
	config *cluster.ContainerConfig,
	need []structs.DiskStorage,
	skipSAN bool) error {

	logrus.Debugf("Engine %s alloc storage %v", engine.Addr, need)

	dc, node, err := gd.getNode(engine.ID)
	if err != nil {
		return err
	}

	sys, err := gd.systemConfig()
	if err != nil {
		return err
	}

	// add localtime
	config.HostConfig.Binds = append(config.HostConfig.Binds, "/etc/localtime:/etc/localtime:ro")

	for i := range need {
		if need[i].Type == "nfs" || need[i].Type == "NFS" {
			name := fmt.Sprintf("%s:%s", sys.NFSOption.MountDir, sys.BackupDir)
			config.HostConfig.Binds = append(config.HostConfig.Binds, name)
			continue
		}

		name := fmt.Sprintf("%s_%s_LV", penging.unit.Unit.Name, need[i].Name)

		if storage.IsLocalStore(need[i].Type) {
			lv, err := node.localStorageAlloc(name, penging.unit.Unit.ID, need[i].Type, need[i].Size)
			if err != nil {
				return err
			}
			penging.localStore = append(penging.localStore, lv)
			name = fmt.Sprintf("%s:/DBAAS%s", name, need[i].Name)
			config.HostConfig.Binds = append(config.HostConfig.Binds, name)
			config.HostConfig.VolumeDriver = node.localStore.Driver()

			continue
		}

		if !skipSAN {
			if dc.store == nil {
				return errors.Errorf("Datacenter %s SAN Store is required", dc.Name)
			}

			vgName := penging.unit.Unit.Name + _SAN_VG

			lun, lv, err := dc.store.Alloc(name, penging.unit.Unit.ID, vgName, need[i].Size)
			if lun.ID != "" {
				penging.sanStore = append(penging.sanStore, lun)
				penging.localStore = append(penging.localStore, lv)
			}
			if err != nil {
				return err
			}

			err = dc.store.Mapping(node.ID, vgName, lun.ID)
			if err != nil {
				return err
			}
		}
		name = fmt.Sprintf("%s:/DBAAS%s", name, need[i].Name)
		config.HostConfig.Binds = append(config.HostConfig.Binds, name)
		config.HostConfig.VolumeDriver = dc.store.Driver()

		continue
	}

	return nil
}

func (node *Node) localStorageAlloc(name, unitID, storageType string, size int) (database.LocalVolume, error) {
	lv := database.LocalVolume{}
	if !storage.IsLocalStore(storageType) {
		return lv, errors.Errorf("'%s' storage type isnot '%s'", storageType, storage.LocalStorePrefix)
	}

	if node.localStore == nil {
		return lv, errors.Errorf("Not Found LoaclStorage of Node %s", node.Addr)
	}

	vgName, err := getVGname(node.engine, storageType)
	if err != nil {
		return lv, err
	}

	lv, err = node.localStore.Alloc(name, unitID, vgName, size)
	if err != nil {
		return lv, err
	}

	return lv, nil
}

type localVolume struct {
	size int
	lv   database.LocalVolume
}

type pendingAllocStore struct {
	created    bool
	unit       *unit
	localStore []localVolume
	sanStore   []database.LUN
}

func localVolumeExtend(host string, lv localVolume) error {
	return updateVolume(host, lv.lv)
}

func (gd *Gardener) cancelStoreExtend(pendings []*pendingAllocStore) error {
	if len(pendings) == 0 {
		return nil
	}

	lvs := make([]database.LocalVolume, 0, len(pendings)*3)
	for _, pending := range pendings {
		for _, lv := range pending.localStore {
			lv.lv.Size -= lv.size
			lvs = append(lvs, lv.lv)
		}
	}

	err := database.TxUpdateMultiLocalVolume(lvs)
	if err != nil {
		return err
	}

	gd.Lock()
	for _, pending := range pendings {
		if pending.created {
			// TODO:cancel san VG extend
		}

		for _, lun := range pending.sanStore {
			store, err := storage.GetStore(lun.StorageSystemID)
			if err != nil {
				logrus.Warnf("cancel Store Extend,%+v", err)
				continue
			}

			err = store.DelMapping(lun.ID)
			if err != nil {
				logrus.Warnf("cancel Store Extend,%+v", err)
			}

			err = store.Recycle(lun.ID, 0)
			if err != nil {
				logrus.Warnf("cancel Store Extend,%+v", err)
			}
		}
	}
	gd.Unlock()

	return nil
}

func (node *Node) localStorageExtend(name, storageType string, size int) (database.LocalVolume, error) {
	lv := database.LocalVolume{}

	if !storage.IsLocalStore(storageType) {
		return lv, errors.Errorf("'%s' storage type isnot '%s'", storageType, storage.LocalStorePrefix)
	}
	if node.localStore == nil {
		return lv, errors.Errorf("not found LoaclStorage of Node %s", node.Addr)
	}
	vgName, err := getVGname(node.engine, storageType)
	if err != nil {
		return lv, err
	}

	return node.localStore.Extend(vgName, name, size)
}

func (svc *Service) volumesPendingExpension(gd *Gardener, _type string, extensions []structs.DiskStorage) ([]*pendingAllocStore, error) {
	if len(extensions) == 0 {
		return nil, nil
	}

	units, err := svc.getUnitByType(_type)
	if err != nil {
		return nil, err
	}

	pendings := make([]*pendingAllocStore, 0, len(units))

	for _, u := range units {
		pending, _, err := pendingAllocUnitStore(gd, u, u.EngineID, extensions)
		if pending != nil {
			pendings = append(pendings, pending)
		}
		if err != nil {
			return pendings, err
		}
	}

	return pendings, nil
}

func pendingAllocUnitStore(gd *Gardener, u *unit, engineID string, need []structs.DiskStorage) (*pendingAllocStore, []string, error) {
	dc, node, err := gd.getNode(engineID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "not found node by Engine:"+engineID)
	}

	pending := &pendingAllocStore{
		unit:       u,
		localStore: make([]localVolume, 0, 3),
		sanStore:   make([]database.LUN, 0, 3),
	}
	binds := make([]string, 0, len(need))

	for d := range need {
		if need[d].Type == "NFS" || need[d].Type == "nfs" {
			continue
		}
		name := fmt.Sprintf("%s_%s_LV", u.Name, need[d].Name)

		if storage.IsLocalStore(need[d].Type) && need[d].Size > 0 {
			lv, err := node.localStorageExtend(name, need[d].Type, need[d].Size)
			if err != nil {
				return pending, binds, err
			}
			pending.localStore = append(pending.localStore, localVolume{
				lv:   lv,
				size: need[d].Size,
			})

			name = fmt.Sprintf("%s:/DBAAS%s", name, need[d].Name)
			binds = append(binds, name)

			continue
		}

		if dc.store == nil {
			return pending, binds, errors.Errorf("Datacenter Store required")
		}

		lun, lv, err := dc.store.Extend(name, need[d].Size)
		if lun.ID != "" {
			pending.sanStore = append(pending.sanStore, lun)
			pending.localStore = append(pending.localStore, localVolume{
				lv:   lv,
				size: need[d].Size,
			})
		}
		if err != nil {
			logrus.Errorf("SAN Store Alloc error:%s,%s", err, name)

			return pending, binds, err
		}

		err = dc.store.Mapping(node.ID, lun.VGName, lun.ID)
		if err != nil {
			return pending, binds, err
		}

		name = fmt.Sprintf("%s:/DBAAS%s", name, need[d].Name)
		binds = append(binds, name)
	}

	return pending, binds, err
}

func (svc *Service) handleScaleUp(gd *Gardener, _type string, updateConfig *container.UpdateConfig) ([]pendingContainerUpdate, error) {
	if updateConfig == nil {
		return nil, nil
	}
	ncpu, err := parseCpuset(updateConfig.CpusetCpus)
	if err != nil {
		return nil, err
	}
	need := int64(ncpu)

	units, err := svc.getUnitByType(_type)
	if err != nil {
		return nil, err
	}

	var used int64
	if need > 0 {
		cpuset := units[0].container.Info.HostConfig.CpusetCpus
		used, err = utils.GetCPUNum(cpuset)
		if err != nil {
			return nil, errors.Wrap(err, "parse CpusetCpus:"+cpuset)
		}
	}
	if (need == 0 || used == need) && (updateConfig.Memory == 0 ||
		updateConfig.Memory == units[0].container.Info.HostConfig.Memory) {
		return nil, nil
	}

	for _, u := range units {
		if u.engine.Memory-u.engine.UsedMemory()-updateConfig.Memory+u.container.Config.HostConfig.Memory < 0 {
			return nil, errors.Errorf("Engine %s:%s have not enough memory for Container %s update", u.engine.ID, u.engine.IP, u.Name)
		}
	}

	pendings := make([]pendingContainerUpdate, 0, len(units))

	if need == used || need == 0 {
		for _, u := range units {
			pendings = append(pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				unit:        u,
				engine:      u.engine,
				config:      *updateConfig,
			})
		}
	} else if need < used {
		for _, u := range units {
			cpusetCpus, err := reduceCPUset(u.container.Info.HostConfig.CpusetCpus, int(need))
			if err != nil {
				return nil, err
			}
			pendings = append(pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				cpusetCpus:  cpusetCpus,
				unit:        u,
				engine:      u.engine,
				config:      *updateConfig,
			})
		}
	} else {
		for _, u := range units {
			reserve := make([]string, 0, len(svc.units))
			for _, pending := range pendings {
				if u.engine.ID == pending.engine.ID {
					reserve = append(reserve, pending.cpusetCpus)
				}
			}
			cpusetCpus, err := gd.allocCPUs(u.engine, fmt.Sprintf("%d", need-used), reserve...)
			if err != nil {
				return nil, err
			}
			cpusetCpus = u.container.Info.HostConfig.CpusetCpus + "," + cpusetCpus
			pendings = append(pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				cpusetCpus:  cpusetCpus,
				unit:        u,
				engine:      u.engine,
				config:      *updateConfig,
			})
		}
	}

	return pendings, nil
}

func reduceCPUset(cpusetCpus string, need int) (string, error) {
	cpus, err := utils.ParseUintList(cpusetCpus)
	if err != nil {
		return "", errors.Wrap(err, "parse cpusetCpus:"+cpusetCpus)
	}

	cpuSlice := make([]int, 0, len(cpus))
	for k, ok := range cpus {
		if ok {
			cpuSlice = append(cpuSlice, k)
		}
	}
	sort.Ints(cpuSlice)

	cpuString := make([]string, need)
	for n := 0; n < need; n++ {
		cpuString[n] = strconv.Itoa(cpuSlice[n])
	}

	return strings.Join(cpuString, ","), nil
}

func isSanVG(name string) bool {
	return strings.HasSuffix(name, _SAN_VG)
}
