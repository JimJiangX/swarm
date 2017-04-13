package strategy

import (
	"sort"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
)

// GroupPlacementStrategy places a container on the node with the fewest running containers.
type GroupPlacementStrategy struct {
}

// Initialize a GroupPlacementStrategy.
func (GroupPlacementStrategy) Initialize() error {
	return nil
}

// Name returns the name of the strategy.
func (GroupPlacementStrategy) Name() string {
	return "group"
}

// RankAndSort sorts nodes based on the group strategy applied to the container config.
func (GroupPlacementStrategy) RankAndSort(config *cluster.ContainerConfig, nodes []*node.Node) ([]*node.Node, error) {
	// for group, a healthy node should decrease its weight to increase its chance of being selected
	// set healthFactor to -10 to make health degree [0, 100] overpower cpu + memory (each in range [0, 100])
	const healthFactor int64 = -10
	weightedNodes, err := weighNodes(config, nodes, healthFactor)
	if err != nil {
		return nil, err
	}

	sort.Sort(weightedNodes)

	return byGroup(weightedNodes), nil
}

func byGroup(nodes weightedNodeList) []*node.Node {
	clusters := make(map[string][]*weightedNode)

	for i := range nodes {
		name := nodes[i].Node.Labels["cluster"]

		if group, ok := clusters[name]; ok {
			clusters[name] = append(group, nodes[i])
		} else {
			g := make([]*weightedNode, 1, nodes.Len()-nodes.Len()/2)
			g[0] = nodes[i]
			clusters[name] = g
		}
	}

	list := make([]groupNodes, 0, len(clusters))

	for _, group := range clusters {
		var score int64
		for i := range group {
			score += group[i].Weight
		}

		list = append(list, groupNodes{group, score / int64(len(group))})
	}

	groups := byGroupList(list)

	sort.Sort(groups)

	out := make([]*node.Node, 0, len(nodes))

	for _, list := range groups {
		for j := range list.list {
			out = append(out, list.list[j].Node)
		}
	}

	return out
}

type groupNodes struct {
	list  []*weightedNode
	score int64
}

type byGroupList []groupNodes

func (n byGroupList) Len() int {
	return len(n)
}

func (n byGroupList) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

func (n byGroupList) Less(i, j int) bool {
	var (
		ip = n[i]
		jp = n[j]
	)

	// If the nodes have the same score sort them out by number of nodes.
	if ip.score == jp.score {
		return len(ip.list) < len(jp.list)
	}
	return ip.score < jp.score
}
