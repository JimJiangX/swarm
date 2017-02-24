package resource

import (
	"strconv"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

// volumeDrivers labels
const (
	_SSD                     = "local:SSD"
	_HDD                     = "local:HDD"
	_HDD_VG_Label            = "HDD_VG"
	_SSD_VG_Label            = "SSD_VG"
	_HDD_VG_Size_Label       = "HDD_VG_SIZE"
	_SSD_VG_Size_Label       = "SSD_VG_SIZE"
	defaultFileSystem        = "xfs"
	defaultLocalVolumeDriver = "lvm"
)

type space struct {
	VG     string
	Total  int64
	Free   int64
	Fstype string
}

func (s space) Used() int64 {
	return s.Total - s.Free
}

type volumeDriver interface {
	Driver() string
	Name() string
	Type() string
	Space() space
	Alloc(uid string, require structs.VolumeRequire) (*database.Volume, error)
}

type localVolume struct {
	engine *cluster.Engine
	driver string
	_type  string
	space  space
	vo     database.VolumeOrmer
}

func (lv localVolume) Name() string {
	return lv.space.VG
}

func (lv localVolume) Type() string {
	return lv._type
}

func (lv localVolume) Driver() string {
	return lv.driver
}

func (lv localVolume) Space() space {
	return lv.space
}

func (lv *localVolume) Alloc(uid string, require structs.VolumeRequire) (*database.Volume, error) {
	space := lv.Space()

	volume := database.Volume{
		Size:       require.Size,
		ID:         "",
		Name:       "",
		UnitID:     uid,
		VG:         space.VG,
		Driver:     lv.Driver(),
		Filesystem: space.Fstype,
	}

	err := lv.vo.InsertVolume(volume)
	if err != nil {
		return nil, err
	}

	lv.space.Free -= require.Size

	return &volume, nil
}

type volumeDrivers []volumeDriver

func (vds volumeDrivers) get(_type string) volumeDriver {
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

		if free := driver.Space().Free; free < size {
			return errors.Errorf("volumeDriver %s:%s is not enough free space %d<%d", driver.Name(), typ, free, size)
		}
	}

	return nil
}

func (vds volumeDrivers) AllocVolumes(uid string, stores []structs.VolumeRequire) ([]*database.Volume, error) {
	volumes := make([]*database.Volume, 0, len(stores))

	for i := range stores {
		driver := vds.get(stores[i].Type)
		if driver == nil {
			return volumes, errors.Errorf("")
		}

		v, err := driver.Alloc(uid, stores[i])
		if v != nil {
			volumes = append(volumes, v)
		}
		if err != nil {
			return volumes, err
		}
	}

	return volumes, nil
}

func volumeDriverFromEngine(vo database.VolumeOrmer, e *cluster.Engine, label string) (volumeDriver, error) {
	var vgType, sizeLabel string

	switch label {
	case _HDD_VG_Label:
		vgType = _HDD
		sizeLabel = _HDD_VG_Size_Label

	case _SSD_VG_Label:
		vgType = _SSD
		sizeLabel = _SSD_VG_Size_Label

	default:
	}

	e.RLock()

	vg, ok := e.Labels[label]
	if !ok {
		e.RUnlock()

		return nil, errors.New("not found label by key:" + label)
	}

	size, ok := e.Labels[sizeLabel]
	if ok {
		e.RUnlock()

		return nil, errors.New("not found label by key:" + sizeLabel)
	}

	e.RUnlock()

	total, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return &localVolume{}, errors.Wrapf(err, "parse VG %s:%s", sizeLabel, size)
	}

	lvs, err := vo.ListVolumeByVG(vg)
	if err != nil {
		return nil, err
	}

	var used int64
	for i := range lvs {
		used += lvs[i].Size
	}

	return &localVolume{
		engine: e,
		vo:     vo,
		_type:  vgType,
		driver: defaultLocalVolumeDriver,
		space: space{
			Total:  total,
			Free:   total - used,
			VG:     vg,
			Fstype: defaultFileSystem,
		},
	}, nil
}

func localVolumeDrivers(e *cluster.Engine, vo database.VolumeOrmer) (volumeDrivers, error) {
	drivers := make([]volumeDriver, 0, 4)

	vd, err := volumeDriverFromEngine(vo, e, _HDD_VG_Label)
	if err == nil {
		drivers = append(drivers, vd)
	}

	vd, err = volumeDriverFromEngine(vo, e, _SSD_VG_Label)
	if err == nil {
		drivers = append(drivers, vd)
	}

	return drivers, nil
}
