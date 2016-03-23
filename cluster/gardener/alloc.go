package gardener

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/gardener/database"
)

func (region *Region) allocResource(preAlloc *preAllocResource, engine *cluster.Engine, config cluster.ContainerConfig, Type string) (*cluster.ContainerConfig, error) {
	constraint := fmt.Sprintf("constraint:node==%s", engine.ID)
	config.Env = append(config.Env, constraint)
	config.Hostname = engine.ID
	config.Domainname = engine.Name

	networkings, err := region.getNetworkingSetting(engine, Type, "")
	preAlloc.networkings = append(preAlloc.networkings, networkings...)

	if err != nil {
		return nil, err
	}

	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	for i := range networkings {
		if networkings[i].Type == ContainersNetworking {
			config.Labels[networkingLabelKey] = networkings[i].String()
		} else if networkings[i].Type == ExternalAccessNetworking {
			config.Labels[proxynetworkingLabelKey] = networkings[i].String()
		}
	}

	ncpu, err := parseCpuset(&config)
	if err != nil {
		return nil, err
	}

	// Alloc CPU
	cpuset, err := allocCPUs(engine, ncpu)
	if err != nil {
		return nil, err
	}
	config.Cpuset = cpuset
	config.HostConfig.CpusetCpus = cpuset

	// TODO:Alloc Volume
	bind, err := region.allocStorage(engine, "", "", 0)
	config.HostConfig.Binds = append(config.HostConfig.Binds, bind)

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
	swarmID          string
	networkings      []IPInfo
	pendingContainer *pendingContainer
}

func newPreAllocResource() *preAllocResource {
	return &preAllocResource{
		networkings: make([]IPInfo, 0, 2),
	}
}

func (pre *preAllocResource) consistency() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("preAllocResource consistency Panic:%v;%v", r, err)
		}
	}()

	db, err := database.GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ipTables := make([]database.IPStatus, len(pre.networkings))
	for i := range pre.networkings {
		ipTables[i].Allocated = true
		ipTables[i].IP = pre.networkings[i].ipuint32
		ipTables[i].Prefix = pre.networkings[i].Prefix
	}

	err = database.TxUpdateMultiIPStatue(tx, ipTables)
	if err != nil {
		return err
	}

	err = database.TxUPdatePorts(tx, pre.unit.ports)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Region) Recycle(pendings []*preAllocResource) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Panic:%v;%s", r, err)
		}
	}()

	r.scheduler.Lock()
	for i := range pendings {
		if pendings[i] == nil {
			continue
		}

		swarmID := pendings[i].pendingContainer.Config.SwarmID()
		delete(r.pendingContainers, swarmID)
	}
	r.scheduler.Unlock()

	db, err := database.GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	r.Lock()

	for i := range pendings {
		ipsStatus := r.recycleNetworking(pendings[i])

		database.TxUpdateMultiIPStatue(tx, ipsStatus)

		ports := pendings[i].unit.ports
		for p := range ports {
			ports[p] = database.Port{
				Port:      ports[p].Port,
				Allocated: false,
			}
		}

		database.TxUPdatePorts(tx, ports)
	}

	r.Unlock()

	return tx.Commit()
}

func (r *Region) recycleNetworking(pre *preAllocResource) []database.IPStatus {
	// networking recycle
	ipsStatus := make([]database.IPStatus, 0, 10)

	for i := range pre.networkings {

	loop:
		for n := range r.networkings {

			if r.networkings[n].ID != pre.networkings[i].Networking ||
				r.networkings[n].Prefix != pre.networkings[i].Prefix {

				continue loop
			}

			for ip := range r.networkings[n].pool {

				if r.networkings[n].pool[ip].ip == pre.networkings[i].ipuint32 {
					r.networkings[n].pool[ip].allocated = false

					ipsStatus = append(ipsStatus, database.IPStatus{
						IP:        pre.networkings[i].ipuint32,
						Prefix:    pre.networkings[i].Prefix,
						Allocated: r.networkings[n].pool[ip].allocated,
					})

					break loop
				}
			}
		}
	}

	return ipsStatus
}

func (r *Region) allocStorage(engine *cluster.Engine, driver, Type string, size int64) (string, error) {

	return "", nil
}
