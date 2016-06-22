package swarm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
)

func (gd *Gardener) allocResource(u *unit, engine *cluster.Engine, config *cluster.ContainerConfig) (*pendingAllocResource, error) {
	pending := newPendingAllocResource()
	pending.unit = u

	// constraint := fmt.Sprintf("constraint:node==%s", engine.ID)
	// config.Env = append(config.Env, constraint)
	// conflicting options:hostname and the network mode
	// config.Hostname = engine.ID
	// config.Domainname = engine.Name

	req := u.Requirement()

	err := allocPorts(req.ports, u, config)
	pending.ports = u.ports
	if err != nil {
		logrus.Errorf("alloc ports error:%s,%v", err, req.ports)
		return pending, err
	}

	networkings, err := gd.allocNetworkings(u.ID, engine, req.networkings, config)
	pending.networkings = append(pending.networkings, networkings...)
	if err != nil {
		logrus.Errorf("alloc networkings error:%s", err)
		return pending, err
	}
	u.networkings = networkings

	ncpu, err := parseCpuset(config.HostConfig.CpusetCpus)
	if err != nil {
		logrus.Error(err)

		return pending, err
	}
	// Alloc CPU
	cpuset, err := gd.allocCPUs(engine, ncpu)
	if err != nil {
		logrus.Errorf("Alloc CPU %d Error:%s", ncpu, err)
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
			config.Labels["container_ip"] = ip
			config.Labels[_NetworkingLabelKey] = networkings[i].String()
		} else if networkings[i].Type == _ExternalAccessNetworking {
			config.Labels[_ProxyNetworkingLabelKey] = networkings[i].String()
		}
	}

	return networkings, nil
}

func allocPorts(need []port, u *unit, config *cluster.ContainerConfig) error {
	if length := len(need); length > 0 {
		ports, err := database.SelectAvailablePorts(length)
		if err != nil || len(ports) < length {
			logrus.Errorf("Alloc Ports Error:%v", err)

			return err
		}

		for i := range need {
			ports[i].Name = need[i].name
			ports[i].Proto = need[i].proto
			ports[i].UnitID = u.Unit.ID
			ports[i].UnitName = u.Unit.Name
			ports[i].Allocated = true
		}

		u.ports = ports[0:length]
	}

	portSlice := make([]string, len(u.ports))
	for i := range u.ports {
		portSlice[i] = strconv.Itoa(u.ports[i].Port)
	}

	config.Env = append(config.Env, fmt.Sprintf("PORT=%s", strings.Join(portSlice, ",")))

	return nil
}

func (gd *Gardener) allocCPUs(engine *cluster.Engine, ncpu int, reserve ...string) (string, error) {
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
	pendingContainer *pendingContainer
	swarmID          string
	ports            []database.Port
	networkings      []IPInfo
	localStore       []database.LocalVolume
	sanStore         []string
}

func newPendingAllocResource() *pendingAllocResource {
	return &pendingAllocResource{
		networkings: make([]IPInfo, 0, 2),
		localStore:  make([]database.LocalVolume, 0, 2),
		sanStore:    make([]string, 0, 2),
	}
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

		if pendings[i].unit != nil {
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
			database.TxDeleteVolumes(tx, pendings[i].unit.Unit.ID)
		}

		for _, lv := range pendings[i].localStore {
			database.TxDeleteVolumes(tx, lv.ID)
		}

		gd.Unlock()
		for _, lun := range pendings[i].sanStore {
			dc, err := gd.DatacenterByNode(pendings[i].unit.Unit.EngineID)
			if err != nil || dc == nil || dc.storage == nil {
				continue
			}
			dc.storage.Recycle(lun, 0)
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

func (gd *Gardener) allocStorage(penging *pendingAllocResource, engine *cluster.Engine, config *cluster.ContainerConfig, need []structs.DiskStorage) error {
	dc, node, err := gd.GetNode(engine.ID)
	if err != nil {
		err := fmt.Errorf("Not Found Node %s,Error:%s", engine.Name, err)
		logrus.Error(err)

		return err
	}

	for i := range need {
		if need[i].Type == "nfs" || need[i].Type == "NFS" {
			sys, err := database.GetSystemConfig()
			if err != nil {
				logrus.Errorf("GetSystemConfig error:%s", err)
				return err
			}
			name := fmt.Sprintf("%s/%s:/%s", sys.NFSOption.MountDir, penging.unit.Name, need[i].Name)
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

		if dc.storage == nil {
			return fmt.Errorf("Not Found Datacenter Storage")
		}
		vgName := penging.unit.Unit.Name + "_SAN_VG"

		lunID, err := dc.storage.Alloc(name, penging.unit.Unit.ID, vgName, need[i].Size)
		if err != nil {
			return err
		}
		penging.sanStore = append(penging.sanStore, lunID)

		err = dc.storage.Mapping(node.ID, vgName, lunID)
		if err != nil {
			return err
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
		return lv, fmt.Errorf("'%s' storage type isnot '%s'", storageType, store.LocalStorePrefix)
	}
	if node.localStore == nil {
		return lv, fmt.Errorf("Not Found LoaclStorage of Node %s", node.Addr)
	}

	vgName, err := node.getVGname(storageType)
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
	sanStore   []string
}

func localVolumeExtend(host string, lv localVolume) error {
	return updateVolume(host, lv.lv, lv.size)
}

func (gd *Gardener) cancelStoreExtend(pendings []*pendingAllocStore) error {
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
			dc, err := gd.DatacenterByNode(pending.unit.Unit.ID)
			if err != nil || dc == nil || dc.storage == nil {
				continue
			}
			dc.storage.Recycle(lun, 0)
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
	vgName, err := node.getVGname(storageType)
	if err != nil {
		return lv, err
	}

	lv, err = node.localStore.Extend(vgName, name, size)

	return lv, err
}

func (gd *Gardener) volumesExtension(svc *Service, need []structs.StorageExtension, task database.Task) (err error) {
	var pendings []*pendingAllocStore
	svc.Lock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		svc.Unlock()

		if err == nil {
			task.Status = _StatusTaskDone
			err = svc.updateDescAfterExtension(need)
			if err != nil {
				logrus.Errorf("service %s update Description error:%s", svc.Name, err)
			}
		}

		if err != nil {
			task.Status = _StatusTaskFailed
			task.Errors = err.Error()
			err1 := gd.cancelStoreExtend(pendings)
			if err1 != nil {
				err = fmt.Errorf("%s,%s", err, err1)
			}
			logrus.Error(err)
		}

		err = database.UpdateTaskStatus(&task, task.Status, time.Now(), task.Errors)
		if err != nil {
			logrus.Errorf("task %s update error:%s", task.ID, err)
		}
	}()

	pendings, err = gd.volumesPendingExpension(svc, need)
	if err != nil {
		logrus.Error(err)

		return err
	}
	if len(pendings) == 0 {
		logrus.Info("no need doing volume extension")
		return nil
	}

	for _, pending := range pendings {
		for _, lv := range pending.localStore {
			err = localVolumeExtend(pending.unit.engine.IP, lv)
			if err != nil {
				logrus.Errorf("unit %s update volume error %s", pending.unit.Name, err)
				return err
			}
			logrus.Debugf("unit %s update volume done, %v", pending.unit.Name, lv)
		}
	}

	//TODO: update san store Volumes

	return nil
}

func (gd *Gardener) volumesPendingExpension(svc *Service, need []structs.StorageExtension) ([]*pendingAllocStore, error) {
	pendings := make([]*pendingAllocStore, 0, len(need)*len(svc.units))

	for e := range need {
		units := svc.getUnitByType(need[e].Type)

		for _, u := range units {
			pending := &pendingAllocStore{
				unit:       u,
				localStore: make([]localVolume, 0, 3),
				sanStore:   make([]string, 0, 3),
			}
			pendings = append(pendings, pending)

			for d := range need[e].Extensions {
				dc, node, err := gd.GetNode(u.engine.ID)
				if err != nil {
					err := fmt.Errorf("Not Found Node %s,Error:%s", u.engine.Name, err)
					logrus.Error(err)
					return pendings, err
				}

				name := fmt.Sprintf("%s_%s_LV", u.Name, need[e].Extensions[d].Name)

				if store.IsLocalStore(need[e].Extensions[d].Type) {
					lv, err := node.localStorageExtend(name, need[e].Extensions[d].Type, need[e].Extensions[d].Size)
					if err != nil {
						return pendings, err
					}
					pending.localStore = append(pending.localStore, localVolume{
						lv:   lv,
						size: need[e].Extensions[d].Size,
					})
					name = fmt.Sprintf("%s:/DBAAS%s", name, need[e].Extensions[d].Name)
					continue
				}

				// TODO:fix later
				if dc.storage == nil {
					return pendings, fmt.Errorf("Not Found Datacenter Storage")
				}
				vgName := u.Name + "_SAN_VG"

				lunID, err := dc.storage.Alloc(name, u.ID, vgName, need[e].Extensions[d].Size)
				if err != nil {
					return pendings, err
				}
				pending.sanStore = append(pending.sanStore, lunID)

				err = dc.storage.Mapping(node.ID, vgName, lunID)
				if err != nil {
					return pendings, err
				}
				name = fmt.Sprintf("%s:/DBAAS%s", name, need[e].Extensions[d].Name)
			}
		}
	}

	return pendings, nil
}

func handlePerScaleUpModule(gd *Gardener, svc *Service, module structs.ScaleUpModule, pendings *[]pendingContainerUpdate) error {
	ncpu, err := parseCpuset(module.Config.CpusetCpus)
	if err != nil {
		return err
	}
	need := int64(ncpu)

	units := svc.getUnitByType(module.Type)
	if len(units) == 0 {
		return fmt.Errorf("Not Found unit '%s' In Service %s", module.Type, svc.Name)
	}

	var used int64
	if need > 0 {
		used, err = utils.GetCPUNum(units[0].container.Info.HostConfig.CpusetCpus)
		if err != nil {
			return err
		}
	}
	if (need == 0 || used == need) && (module.Config.Memory == 0 ||
		module.Config.Memory == units[0].container.Info.HostConfig.Memory) {
		return nil
	}

	for _, u := range units {
		if u.engine.Memory-u.engine.UsedMemory()-int64(module.Config.Memory)+u.container.Config.HostConfig.Memory < 0 {
			return fmt.Errorf("Engine %s:%s have not enough Memory for Container %s Update", u.engine.ID, u.engine.IP, u.Name)
		}
	}

	if need == used || need == 0 {
		for _, u := range units {
			*pendings = append(*pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				unit:        u,
				engine:      u.engine,
				config:      module.Config,
			})
		}
	} else if need < used {
		for _, u := range units {
			cpusetCpus, err := reduceCPUset(u.container.Info.HostConfig.CpusetCpus, int(need))
			if err != nil {
				return err
			}
			*pendings = append(*pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				cpusetCPus:  cpusetCpus,
				unit:        u,
				engine:      u.engine,
				config:      module.Config,
			})
		}
	} else {
		for _, u := range units {
			reserve := make([]string, 0, len(svc.units))
			for _, pending := range *pendings {
				if u.engine.ID == pending.engine.ID {
					reserve = append(reserve, pending.cpusetCPus)
				}
			}
			cpusetCpus, err := gd.allocCPUs(u.engine, int(need-used), reserve...)
			if err != nil {
				return err
			}
			cpusetCpus = u.container.Info.HostConfig.CpusetCpus + "," + cpusetCpus
			*pendings = append(*pendings, pendingContainerUpdate{
				containerID: u.container.ID,
				cpusetCPus:  cpusetCpus,
				unit:        u,
				engine:      u.engine,
				config:      module.Config,
			})
		}
	}

	return nil
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
