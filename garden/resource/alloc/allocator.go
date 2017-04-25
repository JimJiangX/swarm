package alloc

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/scheduler/node"
)

type Allocator interface {
	VolumeAllocator

	ListCandidates(clusters, filters []string, stores []structs.VolumeRequire) ([]database.Node, error)

	AlloctCPUMemory(config *cluster.ContainerConfig, node *node.Node, ncpu, memory int64, reserved []string) (string, error)

	AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string, networkings []string, requires []structs.NetDeviceRequire) ([]database.IP, error)

	RecycleResource(ips []database.IP, lvs []database.Volume) error
}

type VolumeAllocator interface {
	IsNodeStoreEnough(engine *cluster.Engine, stores []structs.VolumeRequire) error

	AlloctVolumes(config *cluster.ContainerConfig, uid string, n *node.Node, stores []structs.VolumeRequire) ([]database.Volume, error)

	ExpandVolumes(eng *cluster.Engine, uid string, stores []structs.VolumeRequire) error
}
