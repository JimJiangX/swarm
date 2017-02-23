package resource

import (
	"fmt"
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

func (at allocator) AlloctCPUMemory(config *cluster.ContainerConfig, node *node.Node, cpu, memory int, reserved []string) (string, error) {
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

const (
	_PF_Device_Label = "pf_dev" // "pf_dev":"dev1,10G"

	// "vp_dev_0":"bond0,mac_xxxx,10M,192.168.1.1,255.255.255.0,192.168.3.0,vlan_xxxx"
	_VP_Devices_Prefix = "vp_dev"
)

type netDevice struct {
	bond      string
	mac       string
	bandwidth int // M/s
	ip        string
	mask      string
	gateway   string
	vlan      string
}

// "vp_dev_0":"bond0,mac_xxxx,10M,192.168.1.1,255.255.255.0,192.168.3.0,vlan_xxxx"
func (dev netDevice) String() string {
	return fmt.Sprintf("%s,%s,%dM,%s,%s,%s,%s", dev.bond, dev.mac, dev.bandwidth,
		dev.ip, dev.mask, dev.gateway, dev.vlan)
}

func parseBandwidth(width string) (int, error) {
	if len(width) < 2 {
		return 0, errors.Errorf("illegal args '%s'", width)
	}

	n := 1
	switch width[len(width)-1] {
	case 'G', 'g':
		n = 1024
	case 'M', 'm':
		n = 1
	default:
		return 0, errors.Errorf("parse bandwidth '%s' error", width)
	}

	num, err := strconv.Atoi(strings.TrimSpace(string(width[:len(width)-1])))
	if err != nil {
		return 0, errors.Wrapf(err, "parse bandwidth '%s'", width)
	}

	return num * n, nil
}

func parseNetDevice(e *cluster.Engine) (map[string]netDevice, int, error) {

	if e == nil || len(e.Labels) == 0 {
		return nil, 0, nil
	}

	devm := make(map[string]netDevice, len(e.Labels))
	total := 0

	for key, val := range e.Labels {
		if key == _PF_Device_Label {
			i := strings.LastIndexByte(val, ',')
			n, err := parseBandwidth(string(val[i+1:]))
			if err != nil {
				return nil, 0, err
			}

			total = n

		} else if strings.HasPrefix(key, _VP_Devices_Prefix) {
			parts := strings.Split(val, ",")
			if len(parts) >= 2 {
				devm[parts[0]] = netDevice{
					bond: parts[0],
					mac:  parts[1],
				}
			}
		}
	}

	for _, c := range e.Containers() {

		for key, val := range c.Labels {
			if !strings.HasPrefix(key, _VP_Devices_Prefix) {
				continue
			}

			parts := strings.Split(val, ",")
			if len(parts) < 7 {
				continue
			}

			_, exist := devm[parts[0]]
			if !exist {
				continue
			}

			n, err := parseBandwidth(parts[2])
			if err != nil {
				delete(devm, parts[0])
				continue
			}

			devm[parts[0]] = netDevice{
				bond:      parts[0],
				mac:       parts[1],
				bandwidth: n,
				ip:        parts[3],
				mask:      parts[4],
				gateway:   parts[5],
				vlan:      parts[6],
			}
		}
	}

	return devm, total, nil
}

func (at allocator) AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string,
	requires []structs.NetDeviceRequire) ([]database.IP, error) {

	engine := at.cluster.Engine(engineID)
	if engine == nil {
		return nil, errors.New("Engine not found")
	}

	devm, width, err := parseNetDevice(engine)
	if err != nil {
		return nil, err
	}

	for _, dev := range devm {
		if dev.bandwidth > 0 {
			width = width - dev.bandwidth
		}
	}

	index, nic := 0, 0
	in := make([]database.NetworkingRequire, 0, len(requires))
	nm := make(map[string]int, len(requires))
	for _, req := range requires {
		if req.Bandwidth > 0 {
			width = width - req.Bandwidth
			nic++
		}

		if v, exist := nm[req.Networking]; exist && index > v {
			in[v].Num++
		} else {
			nm[req.Networking] = index
			in = append(in, database.NetworkingRequire{
				Networking: req.Networking,
				Num:        1,
			})
			index++
		}
	}

	ready := make([]netDevice, 0, nic)
	for _, dev := range devm {
		if dev.ip == "" {
			nic--
			ready = append(ready, dev)
		}
		if nic == 0 {
			break
		}
	}

	// check network device bandwidth and band
	if width <= 0 || nic > 0 {
		return nil, errors.Errorf("Engine:%s not enough Bandwidth for require", engine.Addr)
	}

	networkings, err := at.ormer.AllocNetworking(in, unitID)
	if err != nil {
		return nil, err
	}

	used := make(map[uint32]struct{}, len(requires))
	for i, req := range requires {

	ips:
		for n := range networkings {
			if _, exist := used[networkings[n].IPAddr]; exist {
				continue ips
			}
			if req.Networking == networkings[n].Networking {
				ready[i] = netDevice{
					bond:      ready[i].bond,
					mac:       ready[i].mac,
					bandwidth: req.Bandwidth,
					ip:        utils.Uint32ToIP(networkings[n].IPAddr).String(),
					mask:      strconv.Itoa(networkings[n].Prefix),
					gateway:   networkings[n].Gateway,
					vlan:      networkings[n].VLAN,
				}

				used[networkings[n].IPAddr] = struct{}{}
			}
		}
	}

	for i := range ready {
		key := fmt.Sprintf("%s_0%d", _VP_Devices_Prefix, i)
		config.Config.Labels[key] = ready[i].String()
	}

	return networkings, nil
}
