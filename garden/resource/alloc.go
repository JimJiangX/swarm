package resource

import (
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
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

func (at allocator) ListCandidates(clusters, filters []string, _type string, stores []structs.VolumeRequire) ([]database.Node, error) {
	nodes, err := at.ormer.ListNodesByClusters(clusters, _type, true)
	if err != nil {
		return nil, err
	}

	out := make([]database.Node, 0, len(nodes))

nodes:
	for i := range nodes {
		if nodes[i].Status != statusNodeEnable || nodes[i].EngineID == "" {
			continue
		}

		for f := range filters {
			if nodes[i].ID == filters[f] {
				continue nodes
			}
		}

		engine := at.cluster.Engine(nodes[i].EngineID)
		if engine == nil || !engine.IsHealthy() {
			continue nodes
		}

		ok, err := at.isNodeStoreEnough(engine, stores)
		if !ok || err != nil {
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

	// TODO:third-part volumeDrivers

	return drivers, nil
}

func (at allocator) AlloctVolumes(uid string, n *node.Node, stores []structs.VolumeRequire) ([]volume.VolumesCreateBody, error) {
	engine := at.cluster.EngineByAddr(n.Addr)
	if engine == nil {
		return nil, errors.Errorf("not found EngineByAddr:%s from cluster", n.Addr)
	}

	drivers, err := at.findNodeVolumeDrivers(engine)
	if err != nil {
		return nil, err
	}

	err = drivers.isSpaceEnough(stores)
	if err != nil {
		return nil, err
	}

	lvs, err := drivers.AllocVolumes(uid, stores)
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
				"vgname": lvs[i].VGName,
			},
		}
	}

	return volumes, nil
}

func (at allocator) AlloctCPUMemory(node *node.Node, cpu, memory int, reserved []string) (string, error) {
	if node.TotalCpus-node.UsedCpus < int64(cpu) {
		return "", errors.New("")
	}

	if node.TotalMemory-node.UsedMemory < int64(memory) {
		return "", errors.New("")
	}

	containers := node.Containers
	used := make([]string, 0, len(containers)+len(reserved))
	used = append(used, reserved...)
	for i := range containers {
		used = append(used, containers[i].Config.HostConfig.CpusetCpus)
	}

	return findIdleCPUs(used, int(node.TotalCpus), cpu)
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

	if len(cpuSlice) < need {
		return cpusetCpus, errors.Errorf("%s is shortage for need %d", cpusetCpus, need)
	}

	sort.Ints(cpuSlice)

	cpuString := make([]string, need)
	for n := 0; n < need; n++ {
		cpuString[n] = strconv.Itoa(cpuSlice[n])
	}

	return strings.Join(cpuString, ","), nil
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

func (at allocator) AlloctNetworking(id, _type string, num int) (string, error) {
	return "", nil
}
