package resource

import (
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/nic"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
)

type allocator struct {
	ormer   database.Ormer
	cluster cluster.Cluster
}

func NewAllocator(ormer database.Ormer, cluster cluster.Cluster) allocator {
	return allocator{
		ormer:   ormer,
		cluster: cluster,
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

		engine := at.cluster.Engine(nodes[i].EngineID)
		if engine == nil || !engine.IsHealthy() {
			continue nodes
		}

		ok, err := at.isNodeStoreEnough(engine, stores)
		if !ok || err != nil {
			logrus.Debugf("node %s %t %+v", nodes[i].Addr, ok, err)
			continue
		}

		out = append(out, nodes[i])
	}

	return out, nil
}

func (at allocator) isNodeStoreEnough(engine *cluster.Engine, stores []structs.VolumeRequire) (bool, error) {
	drivers, err := at.findNodeVolumeDrivers(engine)
	if err != nil {
		return false, err
	}

	err = drivers.isSpaceEnough(stores)

	return err == nil, err
}

func (at allocator) findNodeVolumeDrivers(engine *cluster.Engine) (volumeDrivers, error) {
	if engine == nil {
		return nil, errors.New("Engine is required")
	}

	drivers, err := localVolumeDrivers(engine, at.ormer)
	if err != nil {
		return nil, err
	}

	nd, err := newNFSDriver(at.ormer, engine.ID)
	if err != nil {
		return nil, err
	}

	drivers = append(drivers, nd)

	// TODO:third-part volumeDrivers

	return drivers, nil
}

func (at allocator) AlloctVolumes(config *cluster.ContainerConfig, uid string, n *node.Node, stores []structs.VolumeRequire) ([]volume.VolumesCreateBody, error) {
	engine := at.cluster.Engine(n.ID)
	if engine == nil {
		return nil, errors.Errorf("not found Engine by ID:%s from cluster", n.Addr)
	}

	drivers, err := at.findNodeVolumeDrivers(engine)
	if err != nil {
		return nil, err
	}

	err = drivers.isSpaceEnough(stores)
	if err != nil {
		return nil, err
	}

	lvs, err := drivers.AllocVolumes(config, uid, stores)
	if err != nil {
		return nil, err
	}

	volumes := make([]volume.VolumesCreateBody, len(lvs))

	for i := range lvs {
		volumes[i] = volume.VolumesCreateBody{
			Name:   lvs[i].Name,
			Driver: lvs[i].Driver,
			Labels: nil,
			DriverOpts: map[string]string{
				"size":   strconv.Itoa(int(lvs[i].Size)),
				"fstype": lvs[i].Filesystem,
				"vgname": lvs[i].VG,
			},
		}
	}

	return volumes, nil
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

func (at allocator) RecycleResource() error {
	return nil
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
	candidates []string, requires []structs.NetDeviceRequire) ([]database.IP, error) {

	engine := at.cluster.Engine(engineID)
	if engine == nil {
		return nil, errors.New("Engine not found")
	}

	used, err := at.ormer.ListIPByEngine(engine.ID)
	if err != nil {
		return nil, err
	}

	devm, width, err := nic.ParseTotalDevice(engine)
	if err != nil {
		return nil, err
	}

	for i := range used {
		if used[i].Bandwidth > 0 {
			width = width - used[i].Bandwidth

			delete(devm, used[i].Bond)
		}
	}

	vnic := 0
	for _, req := range requires {
		if req.Bandwidth > 0 {
			width = width - req.Bandwidth
			vnic++
		}
	}

	ready := make([]nic.Device, 0, vnic)
	for _, dev := range devm {
		if dev.IP == "" {
			vnic--
			ready = append(ready, dev)
		}
		if vnic == 0 {
			break
		}
	}

	// check network device bandwidth and band
	if width <= 0 || vnic > 0 {
		return nil, errors.Errorf("Engine:%s not enough Bandwidth for require", engine.Addr)
	}

	index := 0
	in := make([]database.NetworkingRequire, 0, len(requires))
	for i := range requires {
		if requires[i].Bandwidth > 0 {
			in = append(in, database.NetworkingRequire{
				Bond:      ready[index].Bond,
				Bandwidth: ready[index].Bandwidth,
			})
			index++
		}
	}

	var out []database.IP
	for i := range candidates {
		for l := range in {
			in[l].Networking = candidates[i]
		}
		out, err = at.ormer.AllocNetworking(unitID, engine.ID, in)
		if err == nil {
			break
		}
	}

	if len(out) != len(in) {
		n := 0
	loop:
		for i := range in {
			for _, c := range candidates {
				in[i].Networking = c
				n++

				if n == len(in) {
					break loop
				}
			}
		}

		out, err = at.ormer.AllocNetworking(unitID, engine.ID, in)
		if err != nil {
			return nil, err
		}
	}

	config.HostConfig.NetworkMode = "none"

	return out, nil
}
