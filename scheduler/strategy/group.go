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
	list := make([]groupNodes, 0, 5)

	for i := range nodes {
		found := false
		label := nodes[i].Node.Labels["cluster"]

		for l := range list {
			if list[l].cluster == label {
				list[l].list = append(list[l].list, nodes[i])
				found = true
				break
			}
		}

		if !found {
			glist := make([]*weightedNode, 1, nodes.Len()-nodes.Len()/2)
			glist[0] = nodes[i]

			list = append(list, groupNodes{
				cluster: label,
				list:    glist,
			})
		}
	}

	for _, g := range list {

		for i := range g.list {
			g.score += g.list[i].Weight
		}

		g.score = g.score / int64(len(g.list))
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
	cluster string
	score   int64
	list    []*weightedNode
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
