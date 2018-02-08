package alloc

import (
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc/driver"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
)

func (at *allocator) IsNodeStoreEnough(engine *cluster.Engine, stores []structs.VolumeRequire) error {
	drivers, err := at.findEngineVolumeDrivers(engine)
	if err != nil {
		return err
	}

	return at.isSpaceEnough(drivers, stores)
}

func (at *allocator) findSpace(dv driver.Driver) (driver.Space, error) {
	if dv.Type() != storage.SANStore {
		return dv.Space()
	}

	id := dv.Name()

	if space, ok := at.spaces[id]; ok {
		return space, nil
	}

	space, err := dv.Space()
	if err != nil {
		return space, err
	}

	at.spaces[id] = space

	return space, nil
}

// IsSpaceEnough decide is there enough space for required.
func (at *allocator) isSpaceEnough(vds driver.VolumeDrivers, stores []structs.VolumeRequire) error {
	if len(vds) == 0 {
		return errors.New("not found volume Driver")
	}

	need := make(map[string]int64, len(stores))

	for i := range stores {
		need[stores[i].Type] += stores[i].Size
	}

	for typ, size := range need {

		driver := vds.Get(typ)
		if driver == nil {
			return errors.New("not found volumeDriver by type:" + typ)
		}

		if size == 0 {
			continue
		}

		space, err := at.findSpace(driver)
		if err != nil {
			return err
		}

		if space.Free < size {
			return errors.Errorf("volumeDriver %s:%s is not enough free space %d<%d", driver.Name(), typ, space.Free, size)
		}
	}

	return nil
}

func (at *allocator) AlloctVolumes(config *cluster.ContainerConfig, uid string, n *node.Node, stores []structs.VolumeRequire) ([]database.Volume, error) {
	engine := at.ec.Engine(n.ID)
	if engine == nil {
		return nil, errors.Errorf("not found Engine by ID:%s from cluster", n.Addr)
	}

	drivers, err := at.findEngineVolumeDrivers(engine)
	if err != nil {
		return nil, err
	}

	err = at.isSpaceEnough(drivers, stores)
	if err != nil {
		return nil, err
	}

	lvs, err := drivers.AllocVolumes(config, uid, stores)

	return lvs, err
}

func (at *allocator) ExpandVolumes(engine *cluster.Engine, stores []structs.VolumeRequire) error {
	drivers, err := at.findEngineVolumeDrivers(engine)
	if err != nil {
		return err
	}

	return drivers.ExpandVolumes(stores)
}

func (at *allocator) MigrateVolumes(uid string, old, new *cluster.Engine, lvs []database.Volume) ([]database.Volume, error) {
	if old != nil && old.IsHealthy() {

		drivers, err := at.findEngineVolumeDrivers(old)
		if err != nil {
			return nil, err
		}

		for i := range lvs {
			d := drivers.Get(lvs[i].DriverType)
			if d == nil {
				return nil, errors.New("not found the assigned volumeDriver:" + lvs[i].DriverType)
			}

			err := d.DeactivateVG(lvs[i])
			if err != nil {
				return nil, err
			}
		}
	}

	drivers, err := at.findEngineVolumeDrivers(new)
	if err != nil {
		return nil, err
	}

	out := make([]database.Volume, 0, len(lvs))

	for _, v := range lvs {
		d := drivers.Get(v.DriverType)
		if d == nil {
			return out, errors.New("not found the assigned volumeDriver:" + v.DriverType)
		}

		v.UnitID = uid
		out = append(out, v)

		err := d.ActivateVG(v)
		if err != nil {
			return out, err
		}
	}

	return out, nil
}
