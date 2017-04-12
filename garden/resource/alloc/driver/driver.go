package driver

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

type VolumeIface interface {
	database.GetSysConfigIface

	GetNode(nameOrID string) (database.Node, error)

	InsertVolume(lv database.Volume) error
	GetVolume(nameOrID string) (database.Volume, error)
	ListVolumeByVG(string) ([]database.Volume, error)
	DelVolume(nameOrID string) error

	ListLunByVG(vg string) ([]database.LUN, error)
}

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

func FindEngineVolumeDrivers(iface VolumeIface, engine *cluster.Engine) (VolumeDrivers, error) {
	if engine == nil {
		return nil, errors.New("Engine is required")
	}

	drivers, err := localVolumeDrivers(engine, iface)
	if err != nil {
		return nil, err
	}

	nd, err := newNFSDriver(iface, engine.ID)
	if err != nil {
		return drivers, err
	}
	if nd != nil {
		drivers = append(drivers, nd)
	}

	// SAN Volume Drivers

	node, err := iface.GetNode(engine.ID)
	if err != nil || node.Storage == "" {
		logrus.Debugf("Engine:%s %+v", engine.Name, err)

		return drivers, nil
	}

	sd, err := newSanVolumeDriver(engine, iface, node.Storage)
	if err != nil {
		logrus.Debugf("Engine:%s %+v", engine.Name, err)

		return drivers, err
	}

	if sd != nil {
		drivers = append(drivers, sd)
	}

	return drivers, nil
}

type VolumeDrivers []Driver

func (vds VolumeDrivers) get(_type string) Driver {
	for i := range vds {
		if vds[i].Type() == _type {
			return vds[i]
		}
	}

	return nil
}

func (vds VolumeDrivers) IsSpaceEnough(stores []structs.VolumeRequire) error {
	if len(vds) == 0 {
		return errors.New("not found volume Driver")
	}

	need := make(map[string]int64, len(stores))

	for i := range stores {
		need[stores[i].Type] += stores[i].Size
	}

	for typ, size := range need {

		driver := vds.get(typ)
		if driver == nil {
			return errors.New("not found volumeDriver by type:" + typ)
		}

		space, err := driver.Space()
		if err != nil {
			return err
		}

		if space.Free < size {
			return errors.Errorf("volumeDriver %s:%s is not enough free space %d<%d", driver.Name(), typ, space.Free, size)
		}
	}

	return nil
}

func (vds VolumeDrivers) AllocVolumes(config *cluster.ContainerConfig, uid string, stores []structs.VolumeRequire) ([]database.Volume, error) {
	err := vds.IsSpaceEnough(stores)
	if err != nil {
		return nil, err
	}

	volumes := make([]database.Volume, 0, len(stores))

	for i := range stores {
		driver := vds.get(stores[i].Type)
		if driver == nil {
			return volumes, errors.New("not found the assigned volumeDriver:" + stores[i].Type)
		}

		v, err := driver.Alloc(config, uid, stores[i])
		if v != nil {
			volumes = append(volumes, *v)
		}
		if err != nil {
			return volumes, err
		}
	}

	return volumes, nil
}
