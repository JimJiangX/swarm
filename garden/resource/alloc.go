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

// volumeDrivers labels
const (
	_SSD               = "local:SSD"
	_HDD               = "local:HDD"
	_HDD_VG_Label      = "HDD_VG"
	_SSD_VG_Label      = "SSD_VG"
	_HDD_VG_Size_Label = "HDD_VG_SIZE"
	_SSD_VG_Size_Label = "SSD_VG_SIZE"
	defaultFileSystem  = "xfs"
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

		ok, err := at.isNodeStoreEnough(nodes[i].EngineID, stores)
		if !ok || err != nil {
			continue
		}

		out = append(out, nodes[i])
	}

	return out, nil
}

func (at allocator) isNodeStoreEnough(engineID string, stores []structs.VolumeRequire) (bool, error) {
	engine := at.cluster.Engine(engineID)
	if engine == nil {
		return false, nil
	}

	drivers, err := at.engineVolumeDrivers(engine)
	if err != nil {
		return false, err
	}

	is := drivers.isSpaceEnough(stores)

	return is, nil
}

func volumeDriverFromEngine(e *cluster.Engine, label string) (volumeDriver, error) {
	var vgType, sizeLabel string

	switch label {
	case _HDD_VG_Label:
		vgType = _HDD
		sizeLabel = _HDD_VG_Size_Label

	case _SSD_VG_Label:
		vgType = _SSD
		sizeLabel = _SSD_VG_Size_Label

	default:
	}

	e.RLock()

	vg, ok := e.Labels[label]
	if !ok {
		e.RUnlock()

		return volumeDriver{}, errors.New("not found label by key:" + label)
	}

	size, ok := e.Labels[sizeLabel]
	if ok {
		e.RUnlock()

		return volumeDriver{}, errors.New("not found label by key:" + sizeLabel)
	}

	e.RUnlock()

	total, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return volumeDriver{}, errors.Wrapf(err, "parse VG %s:%s", sizeLabel, size)
	}

	return volumeDriver{
		Total:  total,
		Type:   vgType,
		VG:     vg,
		Fstype: defaultFileSystem,
	}, nil
}

func (at allocator) engineVolumeDrivers(e *cluster.Engine) (volumeDrivers, error) {
	drivers := make([]volumeDriver, 0, 2)

	vd, err := volumeDriverFromEngine(e, _HDD_VG_Label)
	if err == nil {
		drivers = append(drivers, vd)
	}

	vd, err = volumeDriverFromEngine(e, _SSD_VG_Label)
	if err == nil {
		drivers = append(drivers, vd)
	}

	for i := range drivers {

		lvs, err := at.ormer.ListVolumeByVG(drivers[i].VG)
		if err != nil {
			return nil, err
		}

		var used int64
		for i := range lvs {
			used += lvs[i].Size
		}

		if free := drivers[i].Total - used; free < drivers[i].Free {
			drivers[i].Free = free
			drivers[i].Used = used
		}
	}

	return drivers, nil
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

func (at allocator) AlloctVolumes(uid string, n *node.Node, stores []structs.VolumeRequire) ([]volume.VolumesCreateBody, error) {
	engine := at.cluster.Engine(n.ID)
	if engine == nil {
		return nil, errors.Errorf("")
	}

	drivers, err := at.engineVolumeDrivers(engine)
	if err != nil {
		return nil, err
	}

	is := drivers.isSpaceEnough(stores)
	if !is {
		return nil, errors.Errorf("")
	}

	lvs := make([]database.Volume, len(stores))
	for i := range stores {

		driver := drivers.get(stores[i].Type)

		lvs[i] = database.Volume{
			Size:       stores[i].Size,
			ID:         "",
			Name:       "",
			UnitID:     uid,
			VGName:     driver.VG,
			Driver:     driver.Name,
			Filesystem: driver.Fstype,
		}
	}

	err = at.ormer.InsertVolumes(lvs)
	if err != nil {
		return nil, errors.Errorf("")
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

func (at allocator) AlloctNetworking(id, _type string, num int) (string, error) {
	return "", nil
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

type volumeDriver struct {
	Total  int64
	Free   int64
	Used   int64
	Name   string
	Type   string
	VG     string
	Fstype string
}

type volumeDrivers []volumeDriver

func (d volumeDrivers) get(_type string) *volumeDriver {
	for i := range d {
		if d[i].Type == _type {
			return &d[i]
		}
	}

	return nil
}

func (ds volumeDrivers) isSpaceEnough(stores []structs.VolumeRequire) bool {
	need := make(map[string]int64, len(stores))

	for i := range stores {
		need[stores[i].Type] += stores[i].Size
	}

	for typ, size := range need {
		driver := ds.get(typ)
		if driver == nil {
			return false
		}

		if driver.Free < size {
			return false
		}
	}

	return true
}
