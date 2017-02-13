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
	_SSD               = "local:SSD"
	_HDD               = "local:HDD"
	_HDD_VG_Label      = "HDD_VG"
	_SSD_VG_Label      = "SSD_VG"
	_HDD_VG_Size_Label = "HDD_VG_SIZE"
	_SSD_VG_Size_Label = "SSD_VG_SIZE"
	defaultFileSystem  = "xfs"
)

type volumeDriver struct {
	Total  int64
	Free   int64
	Used   int64
	Name   string
	Type   string
	VG     string
	Fstype string
}

type volumeDrivers []volumeDriver

func (d volumeDrivers) get(_type string) *volumeDriver {
	for i := range d {
		if d[i].Type == _type {
			return &d[i]
		}
	}

	return nil
}

func (ds volumeDrivers) isSpaceEnough(stores []structs.VolumeRequire) bool {
	need := make(map[string]int64, len(stores))

	for i := range stores {
		need[stores[i].Type] += stores[i].Size
	}

	for typ, size := range need {
		driver := ds.get(typ)
		if driver == nil {
			return false
		}

		if driver.Free < size {
			return false
		}
	}

	return true
}

func volumeDriverFromEngine(e *cluster.Engine, label string) (volumeDriver, error) {
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

		return volumeDriver{}, errors.New("not found label by key:" + label)
	}

	size, ok := e.Labels[sizeLabel]
	if ok {
		e.RUnlock()

		return volumeDriver{}, errors.New("not found label by key:" + sizeLabel)
	}

	e.RUnlock()

	total, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return volumeDriver{}, errors.Wrapf(err, "parse VG %s:%s", sizeLabel, size)
	}

	return volumeDriver{
		Total:  total,
		Type:   vgType,
		VG:     vg,
		Fstype: defaultFileSystem,
	}, nil
}

func engineVolumeDrivers(e *cluster.Engine, vo database.VolumeOrmer) (volumeDrivers, error) {
	drivers := make([]volumeDriver, 0, 2)

	vd, err := volumeDriverFromEngine(e, _HDD_VG_Label)
	if err == nil {
		drivers = append(drivers, vd)
	}

	vd, err = volumeDriverFromEngine(e, _SSD_VG_Label)
	if err == nil {
		drivers = append(drivers, vd)
	}

	for i := range drivers {

		lvs, err := vo.ListVolumeByVG(drivers[i].VG)
		if err != nil {
			return nil, err
		}

		var used int64
		for i := range lvs {
			used += lvs[i].Size
		}

		if free := drivers[i].Total - used; free < drivers[i].Free {
			drivers[i].Free = free
			drivers[i].Used = used
		}
	}

	return drivers, nil
}
