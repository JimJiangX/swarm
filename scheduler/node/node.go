package node

import (
	"errors"

	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/swarm/cluster"
)

// Node is an abstract type used by the scheduler.
type Node struct {
	ID         string
	IP         string
	Addr       string
	Name       string
	Labels     map[string]string
	Containers cluster.Containers
	Images     []*cluster.Image

	UsedMemory  int64
	UsedCpus    int64
	TotalMemory int64
	TotalCpus   int64

	HealthIndicator int64
}

// NewNode creates a node from an engine.
func NewNode(e *cluster.Engine) *Node {
	return &Node{
		ID:              e.ID,
		IP:              e.IP,
		Addr:            e.Addr,
		Name:            e.Name,
		Labels:          e.Labels,
		Containers:      e.Containers(),
		Images:          e.Images(),
		UsedMemory:      e.UsedMemory(),
		UsedCpus:        e.UsedCpus(),
		TotalMemory:     e.TotalMemory(),
		TotalCpus:       int64(e.TotalCpus()),
		HealthIndicator: e.HealthIndicator(),
	}
}

// IsHealthy responses if node is in healthy state
func (n *Node) IsHealthy() bool {
	return n.HealthIndicator > 0
}

// Container returns the container with IDOrName in the engine.
func (n *Node) Container(IDOrName string) *cluster.Container {
	return n.Containers.Get(IDOrName)
}

// AddContainer_Swarm injects a container into the internal state.
func (n *Node) AddContainer_Swarm(container *cluster.Container) error {
	if container.Config != nil {
		memory := container.Config.HostConfig.Memory
		cpus := container.Config.HostConfig.CPUShares

		if n.TotalMemory-memory < 0 || n.TotalCpus-cpus < 0 {
			return errors.New("not enough resources")
		}
		n.UsedMemory = n.UsedMemory + memory
		n.UsedCpus = n.UsedCpus + cpus
	}
	n.Containers = append(n.Containers, container)
	return nil
}

// AddContainer injects a container into the internal state.
func (n *Node) AddContainer(container *cluster.Container) error {
	if container.Config != nil {
		memory := container.Config.HostConfig.Memory
		cpuset := container.Config.HostConfig.CpusetCpus
		cpus, err := getCPUNum(cpuset)
		if err != nil {
			return err
		}

		if n.TotalMemory-memory < 0 || n.TotalCpus-cpus < 0 {
			return errors.New("not enough resources")
		}
		n.UsedMemory = n.UsedMemory + memory
		n.UsedCpus = n.UsedCpus + cpus
	}
	n.Containers = append(n.Containers, container)
	return nil
}

func getCPUNum(val string) (int64, error) {
	cpus, err := parsers.ParseUintList(val)
	if err != nil {
		return 0, err
	}

	ncpu := int64(0)

	for _, v := range cpus {
		if v {
			ncpu++
		}
	}

	return ncpu, nil
}
