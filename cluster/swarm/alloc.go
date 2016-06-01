package swarm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
)

func (gd *Gardener) allocResource(preAlloc *preAllocResource, engine *cluster.Engine, config *cluster.ContainerConfig) error {
	// TODO:clone config pointer params values
	constraint := fmt.Sprintf("constraint:node==%s", engine.ID)
	config.Env = append(config.Env, constraint)
	// conflicting options:hostname and the network mode
	// config.Hostname = engine.ID
	// config.Domainname = engine.Name

	req := preAlloc.unit.Requirement()

	if length := len(req.ports); length > 0 {
		ports, err := database.SelectAvailablePorts(length)
		if err != nil || len(ports) < length {
			logrus.Errorf("Alloc Ports Error:%v", err)

			return err
		}

		for i := range req.ports {
			ports[i].Name = req.ports[i].name
			ports[i].Proto = req.ports[i].proto
			ports[i].UnitID = preAlloc.unit.Unit.ID
			ports[i].UnitName = preAlloc.unit.Unit.Name
			ports[i].Allocated = true
		}

		preAlloc.unit.ports = ports[0:length]
	}

	portSlice := make([]string, len(preAlloc.unit.ports))
	for i := range preAlloc.unit.ports {
		portSlice[i] = strconv.Itoa(preAlloc.unit.ports[i].Port)
	}
	config.Env = append(config.Env, fmt.Sprintf("PORT=%s", strings.Join(portSlice, ",")))

	networkings, err := gd.getNetworkingSetting(engine, preAlloc.unit.ID, req)
	preAlloc.networkings = append(preAlloc.networkings, networkings...)
	if err != nil {
		logrus.Errorf("Alloc Networking Error:%s", err)

		return err
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

	ncpu, err := parseCpuset(config)
	if err != nil {
		logrus.Error(err)

		return err
	}

	// Alloc CPU
	cpuset, err := gd.allocCPUs(engine, ncpu)
	if err != nil {
		logrus.Errorf("Alloc CPU %d Error:%s", ncpu, err)

		return err
	}

	config.HostConfig.CpusetCpus = cpuset

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

type preAllocResource struct {
	unit             *unit
	pendingContainer *pendingContainer
	swarmID          string
	networkings      []IPInfo
	localStore       []string
	sanStore         []string
}

func newPreAllocResource() *preAllocResource {
	return &preAllocResource{
		networkings: make([]IPInfo, 0, 2),
		localStore:  make([]string, 0, 2),
		sanStore:    make([]string, 0, 2),
	}
}

func (pre *preAllocResource) consistency() (err error) {
	if pre.unit == nil {
		return nil
	}
	tx, err := database.GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = database.TxInsertUnit(tx, pre.unit.Unit)
	if err != nil {
		return err
	}

	err = database.TxUpdatePorts(tx, pre.unit.ports)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (gd *Gardener) Recycle(pendings []*preAllocResource) (err error) {
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
			ports := pendings[i].unit.ports
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

		for _, local := range pendings[i].localStore {
			database.TxDeleteVolumes(tx, local)
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

func (pre *preAllocResource) recycleNetworking() []database.IP {
	// networking recycle
	ips := make([]database.IP, 0, len(pre.networkings)*2)

	for i := range pre.networkings {
		ips = append(ips, database.IP{
			IPAddr:       pre.networkings[i].ipuint32,
			Prefix:       pre.networkings[i].Prefix,
			NetworkingID: pre.networkings[i].Networking,
			UnitID:       "",
			Allocated:    false,
		})
	}

	return ips
}

func (gd *Gardener) allocStorage(penging *preAllocResource, engine *cluster.Engine, config *cluster.ContainerConfig, need []structs.DiskStorage) error {
	dc, err := gd.DatacenterByEngine(engine.ID)
	if err != nil || dc == nil {
		return fmt.Errorf("Not Found Datacenter By Engine,%v", err)
	}

	node, err := dc.GetNode(engine.ID)
	if node == nil || err != nil {
		logrus.Warnf("Not Found Node By Engine ID %s Error:%v", engine.ID, err)

		node, err = gd.GetNode(engine.ID)
		if err != nil {
			err := fmt.Errorf("Not Found Node %s,Error:%s", engine.Name, err)
			logrus.Error(err)

			return err
		}
	}

	for i := range need {
		name := fmt.Sprintf("%s_%s_LV", penging.unit.Unit.Name, need[i].Name)

		if strings.Contains(need[i].Type, store.LocalDiskStore) {
			if node.localStore == nil {
				return fmt.Errorf("Not Found LoaclStorage of Node %s", engine.ID)
			}
			part := strings.SplitN(need[i].Type, ":", 2)
			if len(part) == 1 {
				part = append(part, "HDD")
			}
			vgName := engine.Labels[part[1]+"_VG"]
			if vgName == "" {
				return fmt.Errorf("Not Found VG_Name of %s", need[i].Type)
			}

			lvID, _, err := node.localStore.Alloc(name, penging.unit.Unit.ID, vgName, need[i].Size)
			if err != nil {
				return err
			}

			penging.localStore = append(penging.localStore, lvID)
			name = fmt.Sprintf("%s:/DBAAS%s", name, need[i].Name)
			config.HostConfig.Binds = append(config.HostConfig.Binds, name)
			config.HostConfig.VolumeDriver = node.localStore.Driver()

			continue
		}

		if dc.storage == nil {
			return fmt.Errorf("Not Found Datacenter Storage")
		}
		vgName := penging.unit.Unit.Name + "_SAN_VG"

		lunID, _, err := dc.storage.Alloc(name, penging.unit.Unit.ID, vgName, need[i].Size)
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
