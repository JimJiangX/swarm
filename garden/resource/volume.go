package resource

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/driver"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

type volumeDrivers []driver.Driver

func (vds volumeDrivers) get(_type string) driver.Driver {
	for i := range vds {
		if vds[i].Type() == _type {
			return vds[i]
		}
	}

	return nil
}

func (vds volumeDrivers) isSpaceEnough(stores []structs.VolumeRequire) error {
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

func (vds volumeDrivers) AllocVolumes(config *cluster.ContainerConfig, uid string, stores []structs.VolumeRequire) ([]database.Volume, error) {
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

func (at allocator) isNodeStoreEnough(engine *cluster.Engine, stores []structs.VolumeRequire) (bool, error) {
	drivers, err := driver.FindNodeVolumeDrivers(at.ormer, engine)
	if err != nil {
		return false, err
	}

	vds := volumeDrivers(drivers)

	err = vds.isSpaceEnough(stores)

	return err == nil, err
}
