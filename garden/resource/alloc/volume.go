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

func (at allocator) MigrateVolumes(uid string, config *cluster.ContainerConfig, old, new *cluster.Engine, lvs []database.Volume) error {

	return nil
}
