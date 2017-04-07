package driver

import (
	"fmt"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

// volumeDrivers labels
const (
	_SSD                     = "local:SSD"
	_HDD                     = "local:HDD"
	_HDDVGLabel              = "HDD_VG"
	_SSDVGLabel              = "SSD_VG"
	_HDDVGSizeLabel          = "HDD_VG_SIZE"
	_SSDVGSizeLabel          = "SSD_VG_SIZE"
	defaultFileSystem        = "xfs"
	defaultLocalVolumeDriver = "lvm"
)

func volumeDriverFromEngine(vo database.VolumeOrmer, e *cluster.Engine, label string) (Driver, error) {
	var vgType, sizeLabel string

	switch label {
	case _HDDVGLabel:
		vgType = _HDD
		sizeLabel = _HDDVGSizeLabel

	case _SSDVGLabel:
		vgType = _SSD
		sizeLabel = _SSDVGSizeLabel

	default:
	}

	e.RLock()

	vg, ok := e.Labels[label]
	if !ok || vg == "" {
		e.RUnlock()

		return nil, nil
	}

	size, ok := e.Labels[sizeLabel]
	if !ok {
		e.RUnlock()

		return nil, errors.New("not found label by key:" + sizeLabel)
	}

	e.RUnlock()

	var total int64
	if size != "" {
		t, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			return &localVolume{}, errors.Wrapf(err, "parse VG %s:%s", sizeLabel, size)
		}
		total = t
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
		space: Space{
			Total:  total,
			Free:   total - used,
			VG:     vg,
			Fstype: defaultFileSystem,
		},
	}, nil
}

func localVolumeDrivers(e *cluster.Engine, vo database.VolumeOrmer) ([]Driver, error) {
	drivers := make([]Driver, 0, 4)

	vd, err := volumeDriverFromEngine(vo, e, _HDDVGLabel)
	if err == nil && vd != nil {
		drivers = append(drivers, vd)
	} else {
		logrus.Debugf("%s %s %+v", e.Name, _HDDVGLabel, err)
	}

	vd, err = volumeDriverFromEngine(vo, e, _SSDVGLabel)
	if err == nil && vd != nil {
		drivers = append(drivers, vd)
	} else {
		logrus.Debugf("%s %s %+v", e.Name, _SSDVGLabel, err)
	}

	return drivers, nil
}

type localVolume struct {
	engine *cluster.Engine
	driver string
	_type  string
	space  Space
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

func (lv localVolume) Space() (Space, error) {
	return lv.space, nil
}

func (lv *localVolume) Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error) {
	space, err := lv.Space()
	if err != nil {
		return nil, err
	}

	v := database.Volume{
		Size:       req.Size,
		ID:         utils.Generate32UUID(),
		Name:       fmt.Sprintf("%s_%s_%s_LV", uid[:8], space.VG, req.Name),
		UnitID:     uid,
		VG:         space.VG,
		Driver:     lv.Driver(),
		Filesystem: space.Fstype,
	}

	err = lv.vo.InsertVolume(v)
	if err != nil {
		return nil, err
	}

	lv.space.Free -= req.Size

	name := fmt.Sprintf("%s:/UPM/%s", v.Name, req.Name)
	config.HostConfig.Binds = append(config.HostConfig.Binds, name)
	config.HostConfig.VolumeDriver = lv.Driver()

	return &v, nil
}

func (lv *localVolume) Recycle(v database.Volume) (err error) {
	nameOrID := v.ID
	if nameOrID == "" {
		nameOrID = v.Name
	}
	if nameOrID == "" {
		return nil
	}

	if v.Size <= 0 {
		v, err = lv.vo.GetVolume(nameOrID)
		if err != nil {
			if database.IsNotFound(err) {
				return nil
			}
			return err
		}
	}

	err = lv.vo.DelVolume(nameOrID)
	if err == nil {
		lv.space.Free += v.Size
	}

	return err
}
