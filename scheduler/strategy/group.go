package strategy

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

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
	weightedNodes, err := scoreNodes(config, nodes, healthFactor)
	if err != nil {
		return nil, err
	}

	sort.Sort(weightedNodes)

	out := byGroup(weightedNodes, healthFactor)

	return out, nil
}

func (nodes weightedNodeList) String() string {
	if len(nodes) == 0 {
		return ""
	}

	buf := bytes.NewBuffer(nil)

	for i, n := range nodes {
		buf.WriteString(fmt.Sprintf("%d %s %d\n", i, n.Node.ID, n.Weight))
	}

	return buf.String()
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

func scoreNodes(config *cluster.ContainerConfig, nodes []*node.Node, healthinessFactor int64) (weightedNodeList, error) {
	weightedNodes := weightedNodeList{}

	needCpus := requireOfCPU(config)

	for _, node := range nodes {
		nodeMemory := node.TotalMemory
		nodeCpus := node.TotalCpus

		// Skip nodes that are smaller than the requested resources.
		if nodeMemory < int64(config.HostConfig.Memory) || nodeCpus < config.HostConfig.CPUShares {
			continue
		}

		if nodeMemory-node.UsedMemory < config.HostConfig.Memory || (needCpus > 0 && nodeCpus-node.UsedCpus < needCpus) {
			continue
		}

		var (
			cpuScore    int64 = 100
			memoryScore int64 = 100
		)

		if needCpus > 0 {
			cpuScore = (node.UsedCpus + needCpus) * 100 / nodeCpus
		} else if config.HostConfig.CPUShares > 0 {
			cpuScore = (node.UsedCpus + config.HostConfig.CPUShares) * 100 / nodeCpus
		}

		if config.HostConfig.Memory > 0 {
			memoryScore = (node.UsedMemory + config.HostConfig.Memory) * 100 / nodeMemory
		}

		if cpuScore <= 100 && memoryScore <= 100 {
			weightedNodes = append(weightedNodes, &weightedNode{Node: node, Weight: cpuScore + memoryScore + healthinessFactor*node.HealthIndicator})
		}
	}

	if len(weightedNodes) == 0 {
		return nil, ErrNoResourcesAvailable
	}

	return weightedNodes, nil
}

func byGroup(nodes weightedNodeList, healthFactor int64) []*node.Node {
	list := make([]groupNodes, 0, 5)
loop:
	for i := range nodes {
		if nodes[i] == nil {
			continue
		}

		label := nodes[i].Node.Labels["cluster"]
		if label == "" {
			label = "&&&&&&&"
		}

		for l := range list {
			if list[l].cluster == label {
				list[l].list = append(list[l].list, nodes[i])

				continue loop
			}
		}

		// label is not exist in list,so append it
		glist := make([]*weightedNode, 1, nodes.Len()-nodes.Len()/2)
		glist[0] = nodes[i]

		list = append(list, groupNodes{
			cluster: label,
			list:    glist,
		})

	}

	for _, g := range list {

		for i := range g.list {
			g.score += g.list[i].Weight
		}

		len := int64(len(g.list))

		g.score = g.score/len + healthFactor*len
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
		return len(ip.list) > len(jp.list)
	}

	return ip.score < jp.score
}
