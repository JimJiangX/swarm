package driver

import (
	"strings"

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

type volumeExpandResult struct {
	lv      database.Volume
	lun     database.LUN
	recycle func() error
}

// Driver for volume manage
type Driver interface {
	vgIface

	Driver() string
	Name() string
	Type() string

	Space() (Space, error)

	Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error)

	Expand(volumeID string, size int64) (volumeExpandResult, error)

	Recycle(lv database.Volume) error
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

// AllocVolumes alloc required volume space.
func (vds VolumeDrivers) AllocVolumes(config *cluster.ContainerConfig, uid string, stores []structs.VolumeRequire) ([]database.Volume, error) {
	lvs := make([]database.Volume, 0, len(stores))

	for i := range stores {
		driver := vds.Get(stores[i].Type)
		if driver == nil {
			return lvs, errors.New("not found the assigned volumeDriver:" + stores[i].Type)
		}

		v, err := driver.Alloc(config, uid, stores[i])
		if v != nil {
			lvs = append(lvs, *v)
		}
		if err != nil {
			return lvs, err
		}
	}

	for i := range vds {
		err := vds[i].createVG(lvs)
		if err != nil {
			return lvs, err
		}
	}

	return lvs, nil
}

// ExpandVolumes expand required space for exist volumes
func (vds VolumeDrivers) ExpandVolumes(stores []structs.VolumeRequire) (err error) {
	lvs := make([]database.Volume, 0, len(stores))
	luns := make([]database.LUN, 0, len(stores))

	for i := range stores {
		driver := vds.Get(stores[i].Type)
		if driver == nil {
			return errors.New("not found the assigned volumeDriver:" + stores[i].Type)
		}

		result, _err := driver.Expand(stores[i].ID, stores[i].Size)
		if _err != nil {
			return _err
		}

		defer func(f func() error) {
			if err == nil {
				return
			}

			_err := f()
			if _err != nil {
				err = errors.Errorf("%+v\n%+v", _err, err)
			}
		}(result.recycle)

		if result.lv.ID != "" {
			lvs = append(lvs, result.lv)
		}
		if result.lun.ID != "" {
			luns = append(luns, result.lun)
		}
	}

	for i := range vds {
		err = vds[i].expandVG(luns)
		if err != nil {
			return err
		}
	}

	for i := range lvs {
		d := vds.Get(lvs[i].DriverType)
		err = d.updateVolume(lvs[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func generateVolumeName(uid, tag, name string) string {
	return strings.Join([]string{uid[:8], tag, name}, "_")
}
