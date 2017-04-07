package driver

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
)

type Driver interface {
	Driver() string
	Name() string
	Type() string
	Space() (Space, error)
	Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error)

	Recycle(database.Volume) error
}

type Space struct {
	VG     string
	Total  int64
	Free   int64
	Fstype string
}

func (s Space) Used() int64 {
	return s.Total - s.Free
}
