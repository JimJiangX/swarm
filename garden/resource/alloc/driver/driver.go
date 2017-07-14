package driver

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

// VolumeIface volume alloction record.
type VolumeIface interface {
	localVolumeIface

	database.GetSysConfigIface

	GetNode(nameOrID string) (database.Node, error)
}

// Driver for volume manage
type Driver interface {
	vgIface

	Driver() string
	Name() string
	Type() string

	Space() (Space, error)

	Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error)

	Expand(database.Volume, int64) error

	Recycle(database.Volume) error
}

// Space is VG status
type Space struct {
	VG     string
	Total  int64
	Free   int64
	Fstype string
}

// Used returns the used VG size
func (s Space) Used() int64 {
	return s.Total - s.Free
}

// FindEngineVolumeDrivers returns volume drivers supported by engine.
func FindEngineVolumeDrivers(iface VolumeIface, engine *cluster.Engine) (VolumeDrivers, error) {
	if engine == nil {
		return nil, errors.New("Engine is required")
	}

	sys, err := iface.GetSysConfig()
	if err != nil {
		return nil, err
	}

	drivers, err := localVolumeDrivers(engine, iface, sys.SwarmAgent)
	if err != nil {
		return nil, err
	}

	nd, err := newNFSDriver(iface, engine.ID, sys.SourceDir, sys.BackupDir)
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

	sd, err := newSanVolumeDriver(engine, iface, node.Storage, sys.SwarmAgent)
	if err != nil {
		logrus.Debugf("Engine:%s %+v", engine.Name, err)

		return drivers, err
	}

	if sd != nil {
		drivers = append(drivers, sd)
	}

	return drivers, nil
}

// VolumeDrivers volume drivers.
type VolumeDrivers []Driver

// Get volume driver by type
func (vds VolumeDrivers) Get(_type string) Driver {
	for i := range vds {
		if vds[i].Type() == _type {
			return vds[i]
		}
	}

	return nil
}

// IsSpaceEnough decide is there enough space for required.
func (vds VolumeDrivers) IsSpaceEnough(stores []structs.VolumeRequire) error {
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

// AllocVolumes alloc required volume space.
func (vds VolumeDrivers) AllocVolumes(config *cluster.ContainerConfig, uid string, stores []structs.VolumeRequire) ([]database.Volume, error) {
	err := vds.IsSpaceEnough(stores)
	if err != nil {
		return nil, err
	}

	volumes := make([]database.Volume, 0, len(stores))

	for i := range stores {
		driver := vds.Get(stores[i].Type)
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

// ExpandVolumes expand required space for exist volumes
func (vds VolumeDrivers) ExpandVolumes(uid, agent string, stores []structs.VolumeRequire) error {
	for i := range stores {
		driver := vds.Get(stores[i].Type)
		if driver == nil {
			return errors.New("not found the assigned volumeDriver:" + stores[i].Type)
		}

		space, err := driver.Space()
		if err != nil {
			return err
		}

		// Volume generated as same as Driver.Alloc
		lv := database.Volume{
			Name: fmt.Sprintf("%s_%s_%s_LV", uid[:8], space.VG, stores[i].Name),
		}

		err = driver.Expand(lv, stores[i].Size)
		if err != nil {
			return err
		}
	}

	return nil
}
