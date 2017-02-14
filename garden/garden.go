package garden

import (
	"crypto/tls"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
	"github.com/docker/swarm/scheduler"
	"github.com/docker/swarm/scheduler/node"
)

type allocator interface {
	ListCandidates(clusters, filters []string, _type string, stores []structs.VolumeRequire) ([]database.Node, error)

	AlloctCPUMemory(node *node.Node, cpu, memory int, reserved []string) (string, error)

	AlloctVolumes(id string, n *node.Node, stores []structs.VolumeRequire) ([]volume.VolumesCreateBody, error)

	AlloctNetworking(id, _type string, num int) (string, error)

	RecycleResource() error
}

type Garden struct {
	sync.Mutex
	ormer        database.Ormer
	kvClient     kvstore.Client
	pluginClient pluginapi.PluginAPI

	allocator allocator
	cluster.Cluster
	scheduler  *scheduler.Scheduler
	TLSConfig  *tls.Config
	authConfig *types.AuthConfig
}

func NewGarden(kvc kvstore.Client, cl cluster.Cluster, scheduler *scheduler.Scheduler, ormer database.Ormer, allocator allocator, tlsConfig *tls.Config) *Garden {
	return &Garden{
		// Mutex:       &scheduler.Mutex,
		kvClient:  kvc,
		allocator: allocator,
		Cluster:   cl,
		ormer:     ormer,
		TLSConfig: tlsConfig,
	}
}

func (gd *Garden) KVClient() kvstore.Client {
	return gd.kvClient
}

func (gd *Garden) AuthConfig() (*types.AuthConfig, error) {
	return gd.ormer.GetAuthConfig()
}
