package filter

import (
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
)

// ResourceFilter selects only nodes have enough CPU & memory resource.
type ResourceFilter struct {
}

// Name returns the name of the filter
func (f *ResourceFilter) Name() string {
	return "resource"
}

// Filter is exported
func (f *ResourceFilter) Filter(config *cluster.ContainerConfig, nodes []*node.Node, soft bool) ([]*node.Node, error) {
	var (
		ncpu   = requireOfCPU(config)
		memory = config.HostConfig.Memory
		out    = make([]*node.Node, 0, len(nodes))
	)

	for _, n := range nodes {
		if n.TotalCpus-n.UsedCpus < ncpu {
			continue
		}

		if n.TotalMemory-n.UsedMemory < memory {
			continue
		}

		out = append(out, n)
	}

	return out, nil
}

// GetFilters returns
func (f *ResourceFilter) GetFilters(config *cluster.ContainerConfig) ([]string, error) {
	return nil, nil
}

func requireOfCPU(c *cluster.ContainerConfig) int64 {
	c.HostConfig.CpusetCpus = strings.TrimSpace(c.HostConfig.CpusetCpus)

	if c.HostConfig.CpusetCpus != "" {

		n, err := strconv.ParseInt(c.HostConfig.CpusetCpus, 10, 64)
		if err == nil {
			return n
		}
	}

	return c.HostConfig.CPUShares
}
