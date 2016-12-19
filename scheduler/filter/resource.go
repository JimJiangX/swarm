package filter

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/utils"
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
	ncpu, err := utils.CountCPU(config.HostConfig.CpusetCpus)
	if err != nil {
		return nil, err
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
