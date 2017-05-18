package strategy

import (
	"fmt"
	"testing"

	containertypes "github.com/docker/docker/api/types/container"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler/node"
	"github.com/stretchr/testify/assert"
)

func createContainerConfig(memory int64, cpus string) *cluster.ContainerConfig {
	return cluster.BuildContainerConfig(containertypes.Config{}, containertypes.HostConfig{
		Resources: containertypes.Resources{
			Memory:     memory * 1024 * 1024 * 1024,
			CpusetCpus: cpus,
		},
	}, networktypes.NetworkingConfig{})
}

func TestGroupPlaceDifferentNodeSize(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{
		createNode(fmt.Sprintf("node-0"), 64, 21),
		createNode(fmt.Sprintf("node-1"), 64, 21),
		createNode(fmt.Sprintf("node-2"), 128, 42),
		createNode(fmt.Sprintf("node-3"), 128, 42),
	}

	nodes[0].Labels["cluster"] = "cluster0"
	nodes[1].Labels["cluster"] = "cluster0"
	nodes[2].Labels["cluster"] = "cluster1"
	nodes[3].Labels["cluster"] = "cluster1"

	// add 60 containers
	for i := 0; i < 60; i++ {
		config := createContainerConfig(0, "")
		node := selectTopNode(t, s, config, nodes)
		assert.NoError(t, node.AddContainer(createContainer(fmt.Sprintf("c%d", i), config)))
	}

	assert.Equal(t, 15, len(nodes[0].Containers))
	assert.Equal(t, 15, len(nodes[1].Containers))
	assert.Equal(t, 15, len(nodes[2].Containers))
	assert.Equal(t, 15, len(nodes[3].Containers))
}

func TestGroupPlaceDifferentNodeSizeCPUs(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{
		createNode(fmt.Sprintf("node-0"), 64, 21),
		createNode(fmt.Sprintf("node-1"), 128, 42),
	}

	nodes[0].Labels["cluster"] = "cluster0"
	nodes[1].Labels["cluster"] = "cluster1"

	// add 60 containers 1CPU
	for i := 0; i < 60; i++ {
		config := createContainerConfig(0, "1")
		node := selectTopNode(t, s, config, nodes)
		assert.NoError(t, node.AddContainer(createContainer(fmt.Sprintf("c%d", i), config)))
	}

	assert.Equal(t, 20, len(nodes[0].Containers))
	assert.Equal(t, 40, len(nodes[1].Containers))
}

func TestGroupPlaceEqualWeight(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{}
	for i := 0; i < 2; i++ {
		nodes = append(nodes, createNode(fmt.Sprintf("node-%d", i), 4, 0))
	}

	// add 1 container 2G on node1
	config := createContainerConfig(2, "")
	assert.NoError(t, nodes[0].AddContainer(createContainer("c1", config)))
	assert.Equal(t, int64(2*1024*1024*1024), nodes[0].UsedMemory)

	// add 2 containers 1G on node2
	config = createContainerConfig(1, "")
	assert.NoError(t, nodes[1].AddContainer(createContainer("c2", config)))
	assert.NoError(t, nodes[1].AddContainer(createContainer("c3", config)))
	assert.Equal(t, int64(2*1024*1024*1024), nodes[1].UsedMemory)

	// add another container 1G
	config = createContainerConfig(1, "")
	node := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node.AddContainer(createContainer("c4", config)))
	assert.Equal(t, int64(3*1024*1024*1024), node.UsedMemory)

	// check that the last container ended on the node with the lowest number of containers
	assert.Equal(t, nodes[0].ID, node.ID)
	assert.Equal(t, len(nodes[0].Containers), len(nodes[1].Containers))

}

func TestGroupPlaceContainerMemory(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{}
	for i := 0; i < 2; i++ {
		nodes = append(nodes, createNode(fmt.Sprintf("node-%d", i), 2, 0))
	}

	// add 1 container 1G
	config := createContainerConfig(1, "")
	node1 := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node1.AddContainer(createContainer("c1", config)))
	assert.Equal(t, int64(1024*1024*1024), node1.UsedMemory)

	// add another container 1G
	config = createContainerConfig(1, "")
	node2 := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node2.AddContainer(createContainer("c2", config)))
	assert.Equal(t, int64(1024*1024*1024), node2.UsedMemory)

	// check that both containers ended on different node
	assert.NotEqual(t, node1.ID, node2.ID)
	assert.Equal(t, len(node1.Containers), len(node2.Containers), "")
}

func TestGroupPlaceContainerCPU(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{}
	for i := 0; i < 2; i++ {
		nodes = append(nodes, createNode(fmt.Sprintf("node-%d", i), 0, 2))
	}

	// add 1 container 1CPU
	config := createContainerConfig(0, "1")
	node1 := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node1.AddContainer(createContainer("c1", config)))
	assert.Equal(t, int64(1), node1.UsedCpus)

	// add another container 1CPU
	config = createContainerConfig(0, "1")
	node2 := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node2.AddContainer(createContainer("c2", config)))
	assert.Equal(t, int64(1), node2.UsedCpus)

	// check that both containers ended on different node
	assert.NotEqual(t, node1.ID, node2.ID)
	assert.Equal(t, len(node1.Containers), len(node2.Containers), "")
}

func TestGroupPlaceContainerHuge(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{}
	for i := 0; i < 100; i++ {
		nodes = append(nodes, createNode(fmt.Sprintf("node-%d", i), 1, 1))
	}

	// add 100 container 1CPU
	for i := 0; i < 100; i++ {
		config := createContainerConfig(0, "1")
		node := selectTopNode(t, s, config, nodes)
		assert.NoError(t, node.AddContainer(createContainer(fmt.Sprintf("c%d", i), config)))
	}

	// try to add another container 1CPU
	_, err := s.RankAndSort(createContainerConfig(0, "1"), nodes)
	assert.Error(t, err)

	// add 100 container 1G
	for i := 100; i < 200; i++ {
		config := createContainerConfig(1, "")
		node := selectTopNode(t, s, config, nodes)
		assert.NoError(t, node.AddContainer(createContainer(fmt.Sprintf("c%d", i), config)))
	}

	// try to add another container 1G
	_, err = s.RankAndSort(createContainerConfig(0, "1"), nodes)
	assert.Error(t, err)
}

func TestGroupPlaceContainerOvercommit(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{createNode("node-1", 100, 1)}

	config := createContainerConfig(0, "")

	// Below limit should still work.
	config.HostConfig.Memory = 90 * 1024 * 1024 * 1024
	node := selectTopNode(t, s, config, nodes)
	assert.Equal(t, node, nodes[0])

	// At memory limit should still work.
	config.HostConfig.Memory = 100 * 1024 * 1024 * 1024
	node = selectTopNode(t, s, config, nodes)
	assert.Equal(t, node, nodes[0])

	// Up to 105% it should still work.
	config.HostConfig.Memory = 105 * 1024 * 1024 * 1024
	node = selectTopNode(t, s, config, nodes)
	assert.Equal(t, node, nodes[0])

	// Above it should return an error.
	config.HostConfig.Memory = 106 * 1024 * 1024 * 1024
	_, err := s.RankAndSort(config, nodes)
	assert.Error(t, err)
}

func TestGroupComplexPlacement(t *testing.T) {
	s := &GroupPlacementStrategy{}

	nodes := []*node.Node{}
	for i := 0; i < 2; i++ {
		nodes = append(nodes, createNode(fmt.Sprintf("node-%d", i), 4, 4))
	}

	// add one container 2G
	config := createContainerConfig(2, "")
	node1 := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node1.AddContainer(createContainer("c1", config)))

	// add one container 3G
	config = createContainerConfig(3, "")
	node2 := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node2.AddContainer(createContainer("c2", config)))

	// check that they end up on separate nodes
	assert.NotEqual(t, node1.ID, node2.ID)

	// add one container 1G
	config = createContainerConfig(1, "")
	node3 := selectTopNode(t, s, config, nodes)
	assert.NoError(t, node3.AddContainer(createContainer("c3", config)))

	// check that it ends up on the same node as the 2G
	assert.Equal(t, node1.ID, node3.ID)
}
