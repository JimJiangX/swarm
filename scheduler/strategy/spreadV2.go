package strategy

import (
	"sort"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
)

// SpreadPlacementV2Strategy places a container on the node with the fewest running containers.
type SpreadPlacementV2Strategy struct {
}

// Initialize a SpreadPlacementV2Strategy.
func (p *SpreadPlacementV2Strategy) Initialize() error {
	return nil
}

// Name returns the name of the strategy.
func (p *SpreadPlacementV2Strategy) Name() string {
	return "spread_v2"
}

// RankAndSort sorts nodes based on the spreadV2 strategy applied to the container config.
func (p *SpreadPlacementV2Strategy) RankAndSort(config *cluster.ContainerConfig, nodes []*node.Node) ([]*node.Node, error) {
	// for spread, a healthy node should decrease its weight to increase its chance of being selected
	// set healthFactor to -10 to make health degree [0, 100] overpower cpu + memory (each in range [0, 100])
	const healthFactor int64 = -10
	weightedNodes, err := scoreNodes(config, nodes, healthFactor)
	if err != nil {
		return nil, err
	}

	sort.Sort(weightedNodes)
	output := make([]*node.Node, len(weightedNodes))
	for i, n := range weightedNodes {
		output[i] = n.Node
	}
	return output, nil
}
