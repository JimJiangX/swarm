package driver

import (
	"fmt"
	"time"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/seed/sdk"
	"github.com/pkg/errors"
)

type vgIface interface {
	ActivateVG(v database.Volume) error
	DeactivateVG(v database.Volume) error
}

type unsupportSAN struct{}

var errUnsupportSAN = errors.New("unsupport SAN error")

func (unsupportSAN) ActivateVG(v database.Volume) error {
	return errUnsupportSAN
}

func (unsupportSAN) DeactivateVG(v database.Volume) error {
	return errUnsupportSAN
}

type sanVolume struct {
	iface VolumeIface

	san storage.Store

	engine *cluster.Engine

	port int
}

func newSanVolumeDriver(e *cluster.Engine, iface VolumeIface, storeID string, port int) (Driver, error) {
	stores := storage.DefaultStores()
	san, err := stores.Get(storeID)
	if err != nil {
		return nil, err
	}

	return &sanVolume{
		iface:  iface,
		san:    san,
		engine: e,
		port:   port,
	}, nil
}

func (sv sanVolume) Driver() string {
	return sv.san.Driver()
}

func (sv sanVolume) Name() string {
	return sv.san.ID()
}

func (sv sanVolume) Type() string {
	return storage.SANStore
}

func (sv sanVolume) Space() (Space, error) {
	info, err := sv.san.Info()
	if err != nil {
		return Space{}, err
	}

	return Space{
		//	VG     string
		Total:  info.Total,
		Free:   info.Free,
		Fstype: info.Fstype,
	}, nil
}

func (sv sanVolume) Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error) {
	vg := uid + "_SAN_VG"
	name := generateVolumeName(uid, config.Config.Labels["service.tag"], req.Name)

	lun, lv, err := sv.san.Alloc(name, uid, vg, req.Size)
	if err != nil {
		return nil, err
	}

	lv.EngineID = sv.engine.ID
	lv.UnitID = uid

	err = sv.san.Mapping(sv.engine.ID, vg, lun.ID, uid)
	if err != nil {
		return &lv, err
	}

	err = sv.createSanVG(lv.Name)
	if err != nil {
		return &lv, err
	}

	setVolumeBind(config, lv.Name, req.Name)

	return &lv, nil
}

func (sv sanVolume) Expand(ID string, size int64) (err error) {
	if size <= 0 {
		return nil
	}

	lv, err := sv.iface.GetVolume(ID)
	if err != nil {
		return err
	}

	space, err := sv.Space()
	if err != nil {
		return err
	}

	if space.Free < size {
		return errors.Errorf("node %s local volume driver has no enough space for expansion:%d<%d", sv.engine.IP, space.Free, size)
	}

	lun, lv, err := sv.san.Extend(lv, size)
	if err != nil {
		return err
	}

	defer func() {
		if err == nil {
			return
		}

		_err := sv.recycleLUNs([]database.LUN{lun})
		if _err != nil {
			err = errors.Errorf("recycleLUNs failed,%+v\n%+v", _err, err)
			return
		}

		lv.Size -= size

		_err = sv.iface.SetVolume(lv)
		if _err != nil {
			err = errors.Errorf("recycleLUN success,SetVolume failed\n%+v\n%+v", _err, err)
		}
	}()

	err = sv.san.Mapping(sv.engine.ID, lv.VG, lun.ID, lv.UnitID)
	if err != nil {
		return err
	}

	lun, err = sv.getLunByName(lv.Name, lun.ID)
	if err != nil {
		return err
	}

	agent := fmt.Sprintf("%s:%d", sv.engine.IP, sv.port)

	err = expandSanVG(agent, sv.san.Vendor(), lun)
	if err != nil {
		return err
	}

	return updateVolume(agent, lv)
}

func (sv sanVolume) getLunByName(name, lunID string) (database.LUN, error) {
	luns, err := sv.san.ListLUN(name)
	if err != nil {
		return database.LUN{}, err
	}

	for i := range luns {
		if luns[i].ID == lunID {
			return luns[i], nil

		}
	}

	return database.LUN{}, errors.Errorf("not found LUN:%s", lunID)
}

func (sv sanVolume) ActivateVG(v database.Volume) error {
	luns, err := sv.san.ListLUN(v.Name)
	if err != nil {
		return err
	}

	for i := range luns {
		err = sv.san.Mapping(sv.engine.ID, v.VG, luns[i].ID, v.UnitID)
		if err != nil {
			return err
		}
	}

	luns, err = sv.san.ListLUN(v.Name)
	if err != nil {
		return err
	}

	agent := fmt.Sprintf("%s:%d", sv.engine.IP, sv.port)

	err = sanActivate(agent, v.VG, luns)
	if err != nil {
		return err
	}

	return nil
}

func (sv sanVolume) DeactivateVG(v database.Volume) error {
	luns, err := sv.san.ListLUN(v.Name)
	if err != nil {
		return err
	}

	agent := fmt.Sprintf("%s:%d", sv.engine.IP, sv.port)

	err = sanDeactivate(sv.san.Vendor(), agent, v.VG, luns)
	if err != nil {
		return err
	}

	for i := range luns {
		sv.san.DelMapping(luns[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func (sv sanVolume) recycleLUNs(luns []database.LUN) error {
	for i := range luns {
		if luns[i].MappingTo == "" && luns[i].HostLunID == 0 {
			continue
		}
		err := sv.san.DelMapping(luns[i])
		if err != nil {
			return err
		}
	}

	for i := range luns {
		err := sv.san.RecycleLUN(luns[i].ID, 0)
		if err != nil {
			return err
		}
	}

	return nil
}

func (sv sanVolume) Recycle(lv database.Volume) error {
	luns, err := sv.san.ListLUN(lv.Name)
	if err != nil {
		return err
	}

	agent := fmt.Sprintf("%s:%d", sv.engine.IP, sv.port)
	err = removeSanVG(sv.san.Vendor(), agent, lv.VG, luns)
	if err != nil {
		return err
	}

	return sv.recycleLUNs(luns)
}

func (sv sanVolume) createSanVG(name string) error {
	list, err := sv.san.ListLUN(name)
	if err != nil {
		return err
	}

	agent := fmt.Sprintf("%s:%d", sv.engine.IP, sv.port)

	return createSanVG(agent, sv.san.Vendor(), list)
}

const defaultTimeout = 90 * time.Second

func createSanVG(addr, vendor string, luns []database.LUN) error {
	if len(luns) == 0 {
		return nil
	}

	list, size := make([]int, len(luns)), 0

	for i := range luns {
		list[i] = luns[i].HostLunID
		size += luns[i].SizeByte
	}

	config := sdk.VgConfig{
		HostLunID: list,
		VgName:    luns[0].VG,
		Type:      vendor,
	}

	// TODO:*tls.Config
	client, err := sdk.NewClient(addr, defaultTimeout, nil)
	if err != nil {
		return err
	}

	return client.SanVgCreate(config)
}

func expandSanVG(addr, vendor string, lun database.LUN) error {
	config := sdk.VgConfig{
		HostLunID: []int{lun.HostLunID},
		VgName:    lun.VG,
		Type:      vendor,
	}

	// TODO:*tls.Config
	client, err := sdk.NewClient(addr, defaultTimeout, nil)
	if err != nil {
		return err
	}

	return client.SanVgExtend(config)
}

func updateVolume(addr string, lv database.Volume) error {
	option := sdk.VolumeUpdateOption{
		VgName: lv.VG,
		LvName: lv.Name,
		FsType: lv.Filesystem,
		Size:   int(lv.Size),
	}

	// TODO:*tls.Config
	cli, err := sdk.NewClient(addr, defaultTimeout, nil)
	if err != nil {
		return err
	}

	return cli.VolumeUpdate(option)
}

func sanActivate(addr, vg string, luns []database.LUN) error {
	names := make([]string, len(luns))

	for i := range luns {
		names[i] = luns[i].Name
	}

	opt := sdk.ActiveConfig{
		VgName: vg,
		Lvname: names,
	}

	cli, err := sdk.NewClient(addr, defaultTimeout, nil)
	if err != nil {
		return err
	}

	return cli.SanActivate(opt)
}

func sanDeactivate(vendor, addr, vg string, luns []database.LUN) error {
	names := make([]string, len(luns))
	hls := make([]int, len(luns))

	for i := range luns {
		names[i] = luns[i].Name
		hls[i] = luns[i].HostLunID
	}

	opt := sdk.DeactivateConfig{
		VgName:    vg,
		Lvname:    names,
		HostLunID: hls,
		Vendor:    vendor,
	}

	cli, err := sdk.NewClient(addr, defaultTimeout, nil)
	if err != nil {
		return err
	}

	return cli.SanDeActivate(opt)
}

func removeSanVG(vendor, addr, vg string, luns []database.LUN) error {
	names := make([]string, len(luns))
	hls := make([]int, len(luns))

	for i := range luns {
		names[i] = luns[i].Name
		hls[i] = luns[i].HostLunID
	}

	opt := sdk.RmVGConfig{
		VgName:    vg,
		HostLunID: hls,
		Vendor:    vendor,
	}

	cli, err := sdk.NewClient(addr, defaultTimeout, nil)
	if err != nil {
		return err
	}

	return cli.SanVgRemove(opt)
}

//func removeSCSI(vendor, addr string, luns []database.LUN) error {
//	hls := make([]int, len(luns))

//	for i := range luns {
//		hls[i] = luns[i].HostLunID
//	}

//	opt := sdk.RemoveSCSIConfig{
//		Vendor:    vendor,
//		HostLunId: hls,
//	}

//	cli, err := sdk.NewClient(addr, defaultTimeout, nil)
//	if err != nil {
//		return err
//	}

//	return cli.RemoveSCSI(opt)
//}
