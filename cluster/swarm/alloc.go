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
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
)

func (gd *Gardener) allocResource(u *unit, engine *cluster.Engine, config *cluster.ContainerConfig) (*pendingAllocResource, error) {
	pending := newPendingAllocResource()
	pending.unit = u
	pending.engine = engine

	// constraint := fmt.Sprintf("constraint:node==%s", engine.ID)
	// config.Env = append(config.Env, constraint)
	// conflicting options:hostname and the network mode
	// config.Hostname = engine.ID
	// config.Domainname = engine.Name

	req := u.Requirement()

	ports, portENV, err := allocPorts(req.ports, u.ID, u.Name)
	pending.ports = ports
	if err != nil {
		logrus.Errorf("alloc ports error:%s,%v", err, req.ports)
		return pending, err
	}
	if len(ports) > 0 && portENV != "" {
		config.Env = append(config.Env, portENV)
	}
	u.ports = ports

	networkings, err := gd.allocNetworkings(u.ID, engine, req.networkings, config)
	pending.networkings = append(pending.networkings, networkings...)
	if err != nil {
		logrus.Errorf("alloc networkings error:%s", err)
		return pending, err
	}
	u.networkings = networkings

	// Alloc CPU
	cpuset, err := gd.allocCPUs(engine, config.HostConfig.CpusetCpus)
	if err != nil {
		logrus.Errorf("Alloc CPU '%s' Error:%s", config.HostConfig.CpusetCpus, err)
		return pending, err
	}
	config.HostConfig.CpusetCpus = cpuset

	return pending, nil
}

// unit == unit.ID
func (gd *Gardener) allocNetworkings(unit string, engine *cluster.Engine,
	req []netRequire, config *cluster.ContainerConfig) ([]IPInfo, error) {

	networkings, err := gd.getNetworkingSetting(engine, unit, req)
	if err != nil {
		logrus.Errorf("Alloc Networking Error:%s", err)

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

	ports, err := database.SelectAvailablePorts(length)
	if err != nil || len(ports) < length {
		logrus.Errorf("Alloc Ports Error:%v", err)

		return nil, "", err
	}

	for i := range need {
		ports[i].Name = need[i].name
		ports[i].Proto = need[i].proto
		ports[i].UnitID = unitID
		ports[i].UnitName = unitName
		ports[i].Allocated = true
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
		return "", fmt.Errorf("Engine Alloc CPU Error,%s CPU is Short(%d-%d<%d),", engine.Name, total, used, ncpu)
	}

	containers := engine.Containers()
	list := make([]string, len(reserve), len(reserve)+len(containers)+len(gd.pendingContainers))
	copy(list, reserve)

	for _, c := range containers {
		list = append(list, c.Info.HostConfig.CpusetCpus)
	}

	for _, pending := range gd.pendingContainers {
		if pending.Engine.ID == engine.ID {
			list = append(list, pending.Config.HostConfig.CpusetCpus)
		}
	}

	usedCPUs, err := parseUintList(list)
	if err != nil {
		return "", err
	}

	if total-len(usedCPUs) < ncpu {
		return "", fmt.Errorf("Engine Alloc CPU Error,%s CPU is Short(%d-%d<%d),", engine.Name, total, used, ncpu)
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

func parseUintList(list []string) (map[int]bool, error) {
	if len(list) == 0 {
		return map[int]bool{}, nil
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

	return ints, nil
}

type pendingAllocResource struct {
	unit             *unit
	engine           *cluster.Engine
	pendingContainer *pendingContainer
	swarmID          string
	ports            []database.Port
	networkings      []IPInfo
	localStore       []database.LocalVolume
	sanStore         []database.LUN
}

func newPendingAllocResource() *pendingAllocResource {
	return &pendingAllocResource{
		networkings: make([]IPInfo, 0, 2),
		localStore:  make([]database.LocalVolume, 0, 5),
		sanStore:    make([]database.LUN, 0, 2),
	}
}

func createVolumes(engine *cluster.Engine, lvs []database.LocalVolume, lun []database.LUN) ([]*types.Volume, error) {
	logrus.Debugf("Engine %s create Volumes %d", engine.Addr, len(lvs))
	volumes := make([]*types.Volume, 0, len(lvs))

	for i := range lvs {
		// if volume create on san storage,should created VG before create Volume
		if isSanVG(lvs[i].VGName) {
			err := createSanStoreageVG(engine.IP, lvs[i].Name, lun)
			if err != nil {
				return volumes, errors.Wrapf(err, "create SAN VG:%s ON Engine %s", lvs[i].VGName, engine.IP)
			}
		}

		volume, err := createVolume(engine, lvs[i])
		if err != nil {
			return volumes, err
		}

		volumes = append(volumes, volume)
	}

	return volumes, nil
}

func (pending *pendingAllocResource) consistency() (err error) {
	tx, err := database.GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if pending.unit != nil {
		err = database.TxInsertUnit(tx, pending.unit.Unit)
		if err != nil {
			return err
		}
	}
	err = database.TxUpdatePorts(tx, pending.ports)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (gd *Gardener) Recycle(pendings []*pendingAllocResource) (err error) {
	gd.scheduler.Lock()
	for i := range pendings {
		if pendings[i] == nil ||
			pendings[i].pendingContainer == nil ||
			pendings[i].pendingContainer.Config == nil {

			continue
		}
		swarmID := pendings[i].pendingContainer.Config.SwarmID()
		delete(gd.pendingContainers, swarmID)
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

		if len(pendings[i].networkings) > 0 {
			ips := pendings[i].recycleNetworking()
			database.TxUpdateMultiIPValue(tx, ips)
		}

		if pendings[i].unit != nil && len(pendings[i].ports) > 0 {
			ports := pendings[i].ports
			for p := range ports {
				ports[p].Allocated = false
				ports[p].Name = ""
				ports[p].UnitID = ""
				ports[p].UnitName = ""
				ports[p].Proto = ""
			}
			database.TxUpdatePorts(tx, ports)
			database.TxDeleteUnit(tx, pendings[i].unit.Unit.ServiceID)
			database.TxDeleteVolume(tx, pendings[i].unit.Unit.ID)
		}

		for _, lv := range pendings[i].localStore {
			database.TxDeleteVolume(tx, lv.ID)
		}

		gd.Unlock()
		for _, lun := range pendings[i].sanStore {
			dc, err := gd.DatacenterByEngine(pendings[i].unit.Unit.EngineID)
			if err != nil || dc == nil || dc.storage == nil {
				continue
			}
			dc.storage.DelMapping(lun.ID)
			dc.storage.Recycle(lun.ID, 0)
		}
		gd.Lock()
	}

	return tx.Commit()
}

func (pending *pendingAllocResource) recycleNetworking() []database.IP {
	// networking recycle
	ips := make([]database.IP, 0, len(pending.networkings)*2)

	for i := range pending.networkings {
		ips = append(ips, database.IP{
			IPAddr:       pending.networkings[i].ipuint32,
			Prefix:       pending.networkings[i].Prefix,
			NetworkingID: pending.networkings[i].Networking,
			UnitID:       "",
			Allocated:    false,
		})
	}

	return ips
}

func (gd *Gardener) allocStorage(penging *pendingAllocResource, engine *cluster.Engine,
	config *cluster.ContainerConfig, need []structs.DiskStorage, skipSAN bool) error {

	logrus.Debugf("Engine %s alloc storage %v", engine.Addr, need)

	dc, node, err := gd.GetNode(engine.ID)
	if err != nil {
		err = errors.Errorf("Not Found Node %s,Error:%s", engine.Name, err)
		logrus.Error(err)

		return err
	}

	sys, err := gd.SystemConfig()
	if err != nil {
		temp, err := database.GetSystemConfig()
		if err != nil {
			logrus.Error(err)
			return err
		}
		sys = *temp
	}

	for i := range need {
		if need[i].Type == "nfs" || need[i].Type == "NFS" {
			name := fmt.Sprintf("%s:%s", sys.NFSOption.MountDir, sys.BackupDir)
			config.HostConfig.Binds = append(config.HostConfig.Binds, name)
			continue
		}

		name := fmt.Sprintf("%s_%s_LV", penging.unit.Unit.Name, need[i].Name)

		if store.IsLocalStore(need[i].Type) {
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
			if dc.storage == nil {
				return errors.Errorf("Not Found Datacenter %s SAN Storage", dc.Name)
			}

			vgName := penging.unit.Unit.Name + _SAN_VG

			lun, lv, err := dc.storage.Alloc(name, penging.unit.Unit.ID, vgName, need[i].Size)
			if err != nil {
				return errors.Wrapf(err, "Datacenter %s SAN Alloc Failed", dc.Name)
			}

			penging.sanStore = append(penging.sanStore, lun)
			penging.localStore = append(penging.localStore, lv)

			err = dc.storage.Mapping(node.ID, vgName, lun.ID)
			if err != nil {
				return errors.Wrapf(err, "Datacenter %s SAN Mapping Failed", dc.Name)
			}
		}
		name = fmt.Sprintf("%s:/DBAAS%s", name, need[i].Name)
		config.HostConfig.Binds = append(config.HostConfig.Binds, name)
		config.HostConfig.VolumeDriver = dc.storage.Driver()

		continue
	}

	return nil
}

func (node *Node) localStorageAlloc(name, unitID, storageType string, size int) (database.LocalVolume, error) {
	lv := database.LocalVolume{}
	if !store.IsLocalStore(storageType) {
		return lv, errors.Errorf("'%s' storage type isnot '%s'", storageType, store.LocalStorePrefix)
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
	lv   database.LocalVolume
	size int
}

type pendingAllocStore struct {
	unit       *unit
	localStore []localVolume
	sanStore   []database.LUN
}

func localVolumeExtend(host string, lv localVolume) error {
	return updateVolume(host, lv.lv, lv.size)
}

func (gd *Gardener) cancelStoreExtend(pendings []*pendingAllocStore) error {
	if len(pendings) == 0 {
		return nil
	}
	tx, err := database.GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, pending := range pendings {
		for _, lv := range pending.localStore {
			lv.lv.Size -= lv.size
			err := database.TxUpdateLocalVolume(tx, lv.lv.ID, lv.lv.Size)
			if err != nil {
				return err
			}
		}
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	gd.Lock()
	for _, pending := range pendings {
		for _, lun := range pending.sanStore {
			dc, err := gd.DatacenterByEngine(pending.unit.Unit.EngineID)
			if err != nil || dc == nil || dc.storage == nil {
				continue
			}
			dc.storage.DelMapping(lun.ID)
			dc.storage.Recycle(lun.ID, 0)
		}
	}
	gd.Unlock()

	return nil
}

func (node *Node) localStorageExtend(name, storageType string, size int) (database.LocalVolume, error) {
	lv := database.LocalVolume{}
	if !store.IsLocalStore(storageType) {
		return lv, fmt.Errorf("'%s' storage type isnot '%s'", storageType, store.LocalStorePrefix)
	}
	if node.localStore == nil {
		return lv, fmt.Errorf("Not Found LoaclStorage of Node %s", node.Addr)
	}
	vgName, err := getVGname(node.engine, storageType)
	if err != nil {
		return lv, err
	}

	lv, err = node.localStore.Extend(vgName, name, size)

	return lv, err
}

func (svc *Service) volumesPendingExpension(gd *Gardener, _type string, extensions []structs.DiskStorage) ([]*pendingAllocStore, error) {
	if len(extensions) == 0 {
		return nil, nil
	}

	units, err := svc.getUnitByType(_type)
	if err != nil {
		logrus.WithField("Service", svc.Name).WithError(err).Error("get unit by type")
		return nil, err
	}

	pendings := make([]*pendingAllocStore, 0, len(units))

	for _, u := range units {
		pending, _, err := pendingAllocUnitStore(gd, u, u.EngineID, extensions, false)
		if pending != nil {
			pendings = append(pendings, pending)
		}
		if err != nil {
			return pendings, nil
		}
	}

	return pendings, nil
}

func pendingAllocUnitStore(gd *Gardener, u *unit, engineID string, need []structs.DiskStorage, skipSAN bool) (*pendingAllocStore, []string, error) {
	dc, node, err := gd.GetNode(engineID)
	if err != nil {
		err := fmt.Errorf("Not Found Node %s,Error:%s", engineID, err)
		logrus.Error(err)

		return nil, nil, err
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

		if store.IsLocalStore(need[d].Type) {
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

		if skipSAN {
			continue
		}

		if dc.storage == nil {
			return pending, binds, fmt.Errorf("Not Found Datacenter Storage")
		}
		vgName := u.Name + _SAN_VG

		lun, lv, err := dc.storage.Alloc(name, u.ID, vgName, need[d].Size)
		if err != nil {
			logrus.Errorf("SAN Store Alloc error:%s,%s", err, name)

			return pending, binds, err
		}
		pending.sanStore = append(pending.sanStore, lun)
		pending.localStore = append(pending.localStore, localVolume{
			lv:   lv,
			size: need[d].Size,
		})

		err = dc.storage.Mapping(node.ID, vgName, lun.ID)
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
		used, err = utils.GetCPUNum(units[0].container.Info.HostConfig.CpusetCpus)
		if err != nil {
			return nil, err
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
		return "", err
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
