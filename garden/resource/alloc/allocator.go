package alloc

import (
	"golang.org/x/net/context"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/scheduler/node"
)

// Allocator alloc&recycle hosts/CPU/memory/volumes/networkings resources.
type Allocator interface {
	VolumeAllocator

	NetworkingAllocator

	ListCandidates(clusters, filters []string, stores []structs.VolumeRequire) ([]database.Node, error)

	AlloctCPUMemory(config *cluster.ContainerConfig, node *node.Node, ncpu, memory int64, reserved []string) (string, error)

	RecycleResource(ips []database.IP, lvs []database.Volume) error
}

// NetworkingAllocator networking alloction.
type NetworkingAllocator interface {
	AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string, networkings []string, requires []structs.NetDeviceRequire) ([]database.IP, error)

	AllocDevice(engineID, unitID string, ips []database.IP) ([]database.IP, error)

	UpdateNetworking(ctx context.Context, engineID string, ips []database.IP, width int) error
}

// VolumeAllocator volume alloction.
type VolumeAllocator interface {
	IsNodeStoreEnough(engine *cluster.Engine, stores []structs.VolumeRequire) error

	AlloctVolumes(config *cluster.ContainerConfig, uid string, n *node.Node, stores []structs.VolumeRequire) ([]database.Volume, error)

	ExpandVolumes(eng *cluster.Engine, stores []structs.VolumeRequire) error

	MigrateVolumes(uid string, old, new *cluster.Engine, lvs []database.Volume) ([]database.Volume, error)
}
