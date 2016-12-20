package filter

import (
	"strconv"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
)

type ResourceFilter struct {
}

// Name returns the name of the filter
func (f *ResourceFilter) Name() string {
	return "resource"
}

// Filter is exported
func (f *ResourceFilter) Filter(config *cluster.ContainerConfig, nodes []*node.Node, soft bool) ([]*node.Node, error) {
	var ncpu int64

	if ncpu = config.HostConfig.CPUShares; (ncpu <= 0 || ncpu > 100) &&
		config.HostConfig.CpusetCpus != "" {

		n, err := strconv.ParseInt(config.HostConfig.CpusetCpus, 10, 64)
		if err != nil {
			return nodes, nil
		}

		ncpu = n
	}

	memory := config.HostConfig.Memory
	out := make([]*node.Node, 0, len(nodes))

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
