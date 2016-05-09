package swarm

import (
	"fmt"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
)

func (gd *Gardener) allocResource(preAlloc *preAllocResource, engine *cluster.Engine, config cluster.ContainerConfig, Type string) (*cluster.ContainerConfig, error) {
	constraint := fmt.Sprintf("constraint:node==%s", engine.ID)
	config.Env = append(config.Env, constraint)
	config.Hostname = engine.ID
	config.Domainname = engine.Name

	allocated, need := preAlloc.unit.PortSlice()
	if !allocated && len(need) > 0 {
		ports, err := database.SelectAvailablePorts(len(need))
		if err != nil {
			log.Errorf("Alloc Ports Error:%s", err.Error())

			return nil, err
		}
		for i := range ports {
			ports[i].Name = need[i].name
			ports[i].UnitID = preAlloc.unit.Unit.ID
			ports[i].Proto = need[i].proto
			ports[i].Allocated = true
		}

		preAlloc.unit.ports = ports
	}

	networkings, err := gd.getNetworkingSetting(engine, preAlloc.unit.ID, Type, "")
	preAlloc.networkings = append(preAlloc.networkings, networkings...)
	if err != nil {
		log.Errorf("Alloc Networking Error:%s", err.Error())

		return nil, err
	}

	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	for i := range networkings {
		if networkings[i].Type == _ContainersNetworking {
			config.Labels[_NetworkingLabelKey] = networkings[i].String()
		} else if networkings[i].Type == _ExternalAccessNetworking {
			config.Labels[_ProxyNetworkingLabelKey] = networkings[i].String()
		}
	}

	ncpu, err := parseCpuset(&config)
	if err != nil {
		log.Error(err)

		return nil, err
	}

	// Alloc CPU
	cpuset, err := allocCPUs(engine, ncpu)
	if err != nil {
		log.Errorf("Alloc CPU %d Error:%s", ncpu, err.Error())

		return nil, err
	}

	config.HostConfig.CpusetCpus = cpuset

	return &config, nil
}

func allocCPUs(engine *cluster.Engine, ncpu int) (string, error) {
	total := int(engine.TotalCpus())
	used := int(engine.UsedCpus())
	if ncpu > total-used {
		return "", fmt.Errorf("Engine Alloc CPU Error,%s CPU is Short(%d-%d<%d),", engine.Name, total, used, ncpu)
	}

	return setCPUSets(used, ncpu), nil
}

func setCPUSets(used, ncpu int) string {
	n := int(used)
	cpus := make([]string, ncpu)

	for i := 0; i < ncpu; i++ {
		cpus[i] = strconv.Itoa(n)
		n++
	}

	return strings.Join(cpus, ",")
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
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("preAllocResource consistency Panic:%v;%v", r, err)
		}
	}()

	tx, err := database.GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = database.TxInsertUnit(tx, &pre.unit.Unit)
	if err != nil {
		return err
	}
	/*
		ipTables := make([]database.IP, len(pre.networkings))
		for i := range pre.networkings {
			ipTables[i] = database.IP{
				Allocated:    true,
				UnitID:       pre.unit.ID,
				NetworkingID: pre.networkings[i].Networking,
				IPAddr:       pre.networkings[i].ipuint32,
				Prefix:       pre.networkings[i].Prefix,
			}
		}

		err = database.TxUpdateMultiIPValue(tx, ipTables)
		if err != nil {
			return err
		}
	*/
	err = database.TxUpdatePorts(tx, pre.unit.ports)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (gd *Gardener) Recycle(pendings []*preAllocResource) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Panic:%v;%s", r, err)
		}
	}()

	gd.scheduler.Lock()
	for i := range pendings {
		if pendings[i] == nil {
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

	for i := range pendings {

		ips := gd.recycleNetworking(pendings[i])

		database.TxUpdateMultiIPValue(tx, ips)

		ports := pendings[i].unit.ports
		for p := range ports {
			ports[p] = database.Port{
				Port:      ports[p].Port,
				Allocated: false,
			}
		}

		database.TxUpdatePorts(tx, ports)

		for _, local := range pendings[i].localStore {
			database.DeleteLocalVoume(local)
		}

		for _, lun := range pendings[i].sanStore {
			dc, err := gd.DatacenterByNode(pendings[i].unit.Unit.NodeID)
			if err != nil || dc == nil || dc.storage == nil {
				continue
			}

			dc.storage.Recycle(lun, 0)
		}

		database.TxDelUnit(tx, pendings[i].unit.Unit.ID)
	}

	gd.Unlock()

	return tx.Commit()
}

func (gd *Gardener) recycleNetworking(pre *preAllocResource) []database.IP {
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

	dc.RLock()
	defer dc.RUnlock()
	node := dc.getNode(engine.ID)
	if node == nil {
		return fmt.Errorf("Not Found Node By Engine")
	}

	for i := range need {
		name := fmt.Sprintf("%s_%s_LV:/DBAAS%s", penging.unit.Unit.Name, need[i].Name, need[i].Name)

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

			lunID, _, err := node.localStore.Alloc(name, vgName, need[i].Size)
			if err != nil {
				return err
			}

			penging.localStore = append(penging.localStore, lunID)
			config.HostConfig.Binds = append(config.HostConfig.Binds, name)
			continue
		}

		if dc.storage == nil {
			return fmt.Errorf("Not Found Datacenter Storage")
		}

		lunID, _, err := dc.storage.Alloc(name, need[i].Type, need[i].Size)
		if err != nil {
			return err
		}
		penging.sanStore = append(penging.sanStore, lunID)

		err = dc.storage.Mapping(node.ID, penging.unit.ID, lunID)
		if err != nil {
			return err
		}

		config.HostConfig.Binds = append(config.HostConfig.Binds, name)
		continue
	}

	return nil
}
