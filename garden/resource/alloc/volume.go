package alloc

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc/driver"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/scheduler/node"
	"github.com/pkg/errors"
)

func (at allocator) IsNodeStoreEnough(engine *cluster.Engine, stores []structs.VolumeRequire) error {
	drivers, err := driver.FindEngineVolumeDrivers(at.ormer, engine)
	if err != nil {
		logrus.Warnf("engine:%s find volume drivers,%+v", engine.Name, err)

		if len(drivers) == 0 {
			return err
		}
	}

	err = drivers.IsSpaceEnough(stores)

	return err
}

func (at allocator) AlloctVolumes(config *cluster.ContainerConfig, uid string, n *node.Node, stores []structs.VolumeRequire) ([]database.Volume, error) {
	engine := at.ec.Engine(n.ID)
	if engine == nil {
		return nil, errors.Errorf("not found Engine by ID:%s from cluster", n.Addr)
	}

	drivers, err := driver.FindEngineVolumeDrivers(at.ormer, engine)
	if err != nil {
		logrus.Warnf("engine:%s find volume drivers,%+v", engine.Name, err)

		if len(drivers) == 0 {
			return nil, err
		}
	}

	lvs, err := drivers.AllocVolumes(config, uid, stores)

	return lvs, err
}

func (at allocator) ExpandVolumes(engine *cluster.Engine, uid string, stores []structs.VolumeRequire) error {
	sys, err := at.ormer.GetSysConfig()
	if err != nil {
		return err
	}

	drivers, err := driver.FindEngineVolumeDrivers(at.ormer, engine)
	if err != nil {
		logrus.Warnf("engine:%s find volume drivers,%+v", engine.Name, err)

		if len(drivers) == 0 {
			return err
		}
	}

	agent := fmt.Sprintf("%s:%d", engine.IP, sys.SwarmAgent)

	return drivers.ExpandVolumes(uid, agent, stores)
}

func (at allocator) MigrateVolumes(uid string, old, new *cluster.Engine, lvs []database.Volume) ([]database.Volume, error) {
	drivers, err := driver.FindEngineVolumeDrivers(at.ormer, old)
	if err != nil {
		logrus.Warnf("engine:%s find volume drivers,%+v", old.Name, err)

		if len(drivers) == 0 {
			return nil, err
		}
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

	ndrivers, err := driver.FindEngineVolumeDrivers(at.ormer, new)
	if err != nil {
		logrus.Warnf("engine:%s find volume drivers,%+v", new.Name, err)

		if len(ndrivers) == 0 {
			return nil, err
		}
	}

	out := make([]database.Volume, 0, len(lvs))

	for _, v := range lvs {
		d := ndrivers.Get(v.DriverType)
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
