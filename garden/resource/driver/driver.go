package driver

import (
	"errors"

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

func FindNodeVolumeDrivers(no database.NodeOrmer, engine *cluster.Engine) ([]Driver, error) {
	if engine == nil {
		return nil, errors.New("Engine is required")
	}

	drivers, err := localVolumeDrivers(engine, no)
	if err != nil {
		return nil, err
	}

	nd, err := newNFSDriver(no, engine.ID)
	if err != nil {
		return nil, err
	}
	if nd != nil {
		drivers = append(drivers, nd)
	}

	// TODO:third-part volumeDrivers

	return drivers, nil
}
