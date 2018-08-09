package structs

import "github.com/docker/swarm/cluster"

type Node struct {
	ContainerMax int     `json:"max_container"`
	UsageMax     float32 `json:"usage_max"`

	Tag              string `json:"tag"`
	Addr             string `json:"addr"`
	Storage          string `json:"storage"`
	NetworkPartition string `json:"ha_network_tag"`

	SSHConfig
	NFS

	HDD []string `json:"hdd"`
	SSD []string `json:"ssd"`

	Room string `json:"room,omitempty"`
	Seat string `json:"seat,omitempty"`
}

type NFS struct {
	Address  string `json:"nfs_ip"`
	Dir      string `json:"nfs_dir"`
	MountDir string `json:"nfs_mount_dir"`
	Options  string `json:"nfs_mount_opts"`
}

type SSHConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Port     int    `json:"port,omitempty"` // ssh port
}

type PostNodesRequest []Node

type PostNodeResponse struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
	Task string `json:"task_id"`
}

type NodeInfo struct {
	Enabled      bool    `json:"enabled"`
	UsageMax     float32 `json:"usage_max"`
	ContainerMax int     `json:"max_container"`

	ID               string `json:"id"`
	Tag              string `json:"tag"`
	Room             string `json:"room"`
	Seat             string `json:"seat"`
	NetworkPartition string `json:"ha_network_tag"`

	RegisterAt string `json:"register_at"`

	Engine struct {
		IsHealthy  bool `json:"is_healthy"`
		CPUs       int  `json:"cpus"`
		Memory     int  `json:"memory"`
		FreeCPUs   int  `json:"free_cpus"`
		FreeMemory int  `json:"free_memory"`

		Bandwidth     int      `json:"bandwidth"`      // M/s
		IdleBandwidth int      `json:"idle_bandwidth"` // M/s
		IdleBonds     []string `json:"idle_bonds"`

		ID           string `json:"id"`
		Name         string `json:"name"`
		IP           string `json:"ip"`
		Version      string `json:"server_version"`
		Kernel       string `json:"kernel_version"`
		OS           string `json:"os"`
		OSType       string `json:"os_type"`
		Architecture string `json:"architecture"`
	}

	Containers []container `json:"containers"`

	VolumeDrivers []VolumeDriver `json:"volume_drivers"`
}

type VolumeDriver struct {
	Total  int64  `json:"total"`
	Free   int64  `json:"free"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Type   string `json:"type"`
	VG     string `json:"VG"`
}

type container struct {
	NCPU    int64             `json:"ncpu"`
	Memory  int64             `json:"memory"`
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Command string            `json:"command"`
	Created string            `json:"created"`
	Status  string            `json:"status"`
	State   string            `json:"state"`
	Labels  map[string]string `json:"labels"`
}

func convertToContainer(c *cluster.Container) container {
	if c == nil {
		return container{}
	}

	name := c.Info.Name
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	return container{
		ID:      c.ID,
		Name:    name,
		Image:   c.Image,
		Command: c.Command,
		Created: c.Info.Created,
		Status:  c.Status,
		State:   c.State,
		Labels:  c.Labels,
	}
}

func (n *NodeInfo) SetByEngine(e *cluster.Engine) {
	if e == nil {
		n.Containers = []container{}
		return
	}
	n.Engine.ID = e.ID
	n.Engine.Name = e.Name
	n.Engine.IP = e.IP
	n.Engine.Version = e.Version
	n.Engine.Kernel = e.Labels["kernelversion"]
	n.Engine.OS = e.Labels["operatingsystem"]
	n.Engine.OSType = e.Labels["ostype"]
	n.Engine.Architecture = e.Labels["architecture"]
	n.Engine.CPUs = int(e.Cpus)
	n.Engine.Memory = int(e.Memory)
	n.Engine.IsHealthy = e.IsHealthy()

	var ncpu, memory int64
	containers := e.Containers()
	n.Containers = make([]container, len(containers))

	for i, c := range containers {
		n.Containers[i] = convertToContainer(c)
		if c != nil && c.Config != nil {
			num, err := c.Config.CountCPU()
			if err == nil {
				ncpu += num
				n.Containers[i].NCPU = num
			}

			n.Containers[i].Memory = c.Config.HostConfig.Memory
			memory += c.Config.HostConfig.Memory
		}
	}

	n.Engine.FreeMemory = int(e.Memory - memory)
	n.Engine.FreeCPUs = int(e.Cpus - ncpu)
}
