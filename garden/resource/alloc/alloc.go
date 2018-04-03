package alloc

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc/driver"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type engineCluster interface {
	Engine(IDOrName string) *cluster.Engine
}

type allocatorOrmer interface {
	networkAllocOrmer

	driver.VolumeIface

	ListNodesByClusters(clusters []string, enable bool) ([]database.Node, error)
	RecycleResource(ips []database.IP, lvs []database.Volume) error
}

type allocator struct {
	ormer allocatorOrmer
	ec    engineCluster

	vdsMap map[string]driver.VolumeDrivers
	spaces map[string]driver.Space
}

// NewAllocator is exported.
func NewAllocator(ormer allocatorOrmer, ec engineCluster) Allocator {
	return &allocator{
		ormer:  ormer,
		ec:     ec,
		vdsMap: make(map[string]driver.VolumeDrivers, 20),
		spaces: make(map[string]driver.Space),
	}
}

func (at *allocator) findEngineVolumeDrivers(eng *cluster.Engine) (driver.VolumeDrivers, error) {
	if eng == nil || eng.ID == "" {
		return nil, errors.Errorf("engine is required")
	}

	vds, ok := at.vdsMap[eng.ID]
	if ok && len(vds) > 0 {
		return vds, nil
	}

	vds, err := driver.FindEngineVolumeDrivers(at.ormer, eng)
	if err != nil && len(vds) == 0 {
		return nil, errors.Errorf("engine %s volume drivers error,\n%+v", eng.Name, err)
	}

	at.vdsMap[eng.ID] = vds

	return vds, nil
}

func (at *allocator) ListCandidates(clusters, filters []string, networkings map[string][]string, stores []structs.VolumeRequire) (out []database.Node, err error) {
	netClusters := make([]string, 0, len(clusters))

loop:
	for clusterID, nets := range networkings {

		for i := range nets {

			n, err := at.ormer.CountIPWithCondition(nets[i], false)
			if err == nil && n > 0 {
				netClusters = append(netClusters, clusterID)

				continue loop
			}
		}
	}

	var warning string
	if len(netClusters) == 0 {
		warning = fmt.Sprintf("networkings %s unavailable", networkings)
	}

	nodes, err := at.ormer.ListNodesByClusters(clusters, true)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, errors.Errorf("Warning:%s\n non node in clusters %s", warning, clusters)
	}

	filterMap := make(map[string]struct{}, len(filters))
	for i := range filters {
		filterMap[filters[i]] = struct{}{}
	}

	out = make([]database.Node, 0, len(nodes))
	errs := make([]string, 0, len(nodes)+1)
	if warning != "" {
		errs = append(errs, warning)
	}

	for i := range nodes {
		if !nodes[i].Enabled || nodes[i].EngineID == "" {
			errs = append(errs, fmt.Sprintf("node %s enable=%t,engineID=%s", nodes[i].Addr, nodes[i].Enabled, nodes[i].EngineID))
			continue
		}

		if _, ok := filterMap[nodes[i].ID]; ok {
			errs = append(errs, fmt.Sprintf("node %s is one of filters %s", nodes[i].ID, filters))
			continue
		}

		if _, ok := filterMap[nodes[i].EngineID]; ok {
			errs = append(errs, fmt.Sprintf("node %s is one of filters %s", nodes[i].EngineID, filters))
			continue
		}

		eng := at.ec.Engine(nodes[i].EngineID)
		if eng == nil {
			errs = append(errs, fmt.Sprintf("node:%s not found Engine,%s", nodes[i].Addr, nodes[i].EngineID))
			continue
		}

		if !eng.IsHealthy() {
			errs = append(errs, fmt.Sprintf("node:%s Engine unhealthy", nodes[i].EngineID))
			continue
		}

		if n := len(eng.Containers()); n >= nodes[i].MaxContainer {
			errs = append(errs, fmt.Sprintf("node:%s container num limit(%d>=%d)", nodes[i].EngineID, n, nodes[i].MaxContainer))
			continue
		}

		err := at.IsNodeStoreEnough(eng, stores)
		if err != nil {
			errs = append(errs, fmt.Sprintf("node %s %+v", nodes[i].EngineID, err))
			continue
		}

		out = append(out, nodes[i])
	}

	if len(errs) > 0 {
		err = errors.Errorf("ListCandidates Warnings:%s", errs)
	} else {
		err = nil
	}

	return out, err
}

func (at allocator) AlloctCPUMemory(config *cluster.ContainerConfig, node *node.Node, ncpu, memory int64, reserved []string) (string, error) {
	if free := node.TotalCpus - node.UsedCpus; free < ncpu {
		return "", errors.Errorf("Node:%s CPU is unavailable,%d<%d", node.Addr, free, ncpu)
	}

	if free := node.TotalMemory - node.UsedMemory; free < memory {
		return "", errors.Errorf("Node:%s Memory is unavailable,%d<%d", node.Addr, free, memory)
	}

	var (
		cpuset string
		err    error
	)

	if ncpu > 0 {
		containers := node.Containers
		used := make([]string, 0, len(containers)+len(reserved))
		used = append(used, reserved...)
		for i := range containers {
			used = append(used, containers[i].Config.HostConfig.CpusetCpus)
		}

		cpuset, err = findIdleCPUs(used, int(node.TotalCpus), int(ncpu))
		if err != nil {
			return "", err
		}
	}

	config.HostConfig.Resources.CpusetCpus = cpuset
	config.HostConfig.Resources.Memory = memory
	config.HostConfig.Resources.MemorySwap = int64(float64(memory) * 1.5)

	return cpuset, nil
}

func (at *allocator) RecycleResource(ips []database.IP, lvs []database.Volume) error {
	for i := range lvs {
		eng := at.ec.Engine(lvs[i].EngineID)
		if eng == nil {
			continue
		}

		drivers, err := at.findEngineVolumeDrivers(eng)
		if err != nil {
			return err
		}

		d := drivers.Get(lvs[i].DriverType)
		if d == nil {
			return errors.New("not found volumeDriver by type:" + lvs[i].DriverType)
		} else {
			err := d.Recycle(lvs[i])
			if err != nil {
				return err
			}
		}
	}

	return at.ormer.RecycleResource(ips, lvs)
}

func findIdleCPUs(used []string, total, ncpu int) (string, error) {
	list, err := parseUintList(used)
	if err != nil {
		return "", err
	}

	if total-len(list) < ncpu {
		return "", errors.Errorf("not enough CPU,total=%d,used=%d,required=%d", total, len(list), ncpu)
	}

	free := make([]string, ncpu)
	for i, n := 0, 0; i < total && n < ncpu; i++ {
		if !list[i] {
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
			return ints, errors.Errorf("parseUintList '%s',%s", list[i], err)
		}

		for k, v := range cpus {
			if v {
				ints[k] = v
			}
		}
	}

	return ints, nil
}

func (at allocator) AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string,
	networkings []string, requires []structs.NetDeviceRequire) (out []database.IP, err error) {

	nator := netAllocator{
		ec:    at.ec,
		ormer: at.ormer,
	}

	return nator.AlloctNetworking(config, engineID, unitID, networkings, requires)
}

func (at allocator) AllocDevice(engineID, unitID string, ips []database.IP) ([]database.IP, error) {
	nator := netAllocator{
		ec:    at.ec,
		ormer: at.ormer,
	}

	return nator.AllocDevice(engineID, unitID, ips)
}

func (at allocator) UpdateNetworking(ctx context.Context, engineID string, ips []database.IP, width int) error {
	eng := at.ec.Engine(engineID)
	if eng == nil {
		return errors.Errorf("Engine not found:%s", engineID)
	}

	sys, err := at.ormer.GetSysConfig()
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(eng.IP, strconv.Itoa(sys.Ports.SwarmAgent))

	nator := netAllocator{
		ec:    at.ec,
		ormer: at.ormer,
	}

	return nator.UpdateNetworking(ctx, engineID, addr, ips, width)
}
