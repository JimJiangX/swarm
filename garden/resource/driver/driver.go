package driver

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
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

func FindEngineVolumeDrivers(no database.NodeOrmer, engine *cluster.Engine) ([]Driver, error) {
	if engine == nil {
		return nil, errors.New("Engine is required")
	}

	drivers, err := localVolumeDrivers(engine, no)
	if err != nil {
		return nil, err
	}

	nd, err := newNFSDriver(no, engine.ID)
	if err != nil {
		return drivers, err
	}
	if nd != nil {
		drivers = append(drivers, nd)
	}

	// SAN Volume Drivers

	node, err := no.GetNode(engine.ID)
	if err != nil || node.Storage == "" {
		logrus.Debugf("Engine:%s %+v", engine.Name, err)

		return drivers, nil
	}

	sd, err := newSanVolumeDriver(engine, no, node.Storage)
	if err != nil {
		logrus.Debugf("Engine:%s %+v", engine.Name, err)

		return drivers, err
	}

	if sd != nil {
		drivers = append(drivers, sd)
	}

	return drivers, nil
}
