package alloc

import (
	"golang.org/x/net/context"
	"net"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc/driver"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
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
}

// NewAllocator is exported.
func NewAllocator(ormer allocatorOrmer, ec engineCluster) Allocator {
	return allocator{
		ormer: ormer,
		ec:    ec,
	}
}

func (at allocator) ListCandidates(clusters, filters []string, stores []structs.VolumeRequire) ([]database.Node, error) {
	nodes, err := at.ormer.ListNodesByClusters(clusters, true)
	if err != nil {
		return nil, err
	}

	out := make([]database.Node, 0, len(nodes))

nodes:
	for i := range nodes {
		if !nodes[i].Enabled || nodes[i].EngineID == "" {
			continue
		}

		for f := range filters {
			if nodes[i].ID == filters[f] || nodes[i].EngineID == filters[f] {
				continue nodes
			}
		}

		engine := at.ec.Engine(nodes[i].EngineID)
		if engine == nil || !engine.IsHealthy() {
			continue nodes
		}

		err := at.IsNodeStoreEnough(engine, stores)
		if err != nil {
			logrus.Debugf("node %s %+v", nodes[i].Addr, err)
			continue
		}

		out = append(out, nodes[i])
	}

	return out, nil
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

	return cpuset, nil
}

func (at allocator) RecycleResource(ips []database.IP, lvs []database.Volume) error {
	for i := range lvs {
		eng := at.ec.Engine(lvs[i].EngineID)
		if eng == nil {
			continue
		}

		drivers, err := driver.FindEngineVolumeDrivers(at.ormer, eng)
		if err != nil {
			logrus.Warnf("engine:%s find volume drivers,%+v", eng.Name, err)

			if len(drivers) == 0 {
				continue
			}
		}

		d := drivers.Get(lvs[i].DriverType)
		if d != nil {
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
