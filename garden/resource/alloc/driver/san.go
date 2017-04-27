package driver

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/seed/sdk"
	"github.com/pkg/errors"
)

type sanVolume struct {
	iface VolumeIface

	san storage.Store

	engine *cluster.Engine

	pluginPort string
}

func newSanVolumeDriver(e *cluster.Engine, iface VolumeIface, storeID string) (Driver, error) {
	sys, err := iface.GetSysConfig()
	if err != nil {
		return nil, err
	}

	stores := storage.DefaultStores()
	san, err := stores.Get(storeID)
	if err != nil {
		return nil, err
	}

	return &sanVolume{
		iface:      iface,
		san:        san,
		engine:     e,
		pluginPort: strconv.Itoa(sys.SwarmAgent),
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
	name := fmt.Sprintf("%s_%s_%s_LV", uid, req.Type, req.Name)
	vg := uid + "_SAN_VG"

	lun, lv, err := sv.san.Alloc(name, uid, vg, req.Size)
	if err != nil {
		return nil, err
	}

	err = sv.san.Mapping(sv.engine.ID, vg, lun.ID)
	if err != nil {
		return &lv, err
	}

	err = sv.createSanVG(vg)
	if err != nil {
		return &lv, err
	}

	name = fmt.Sprintf("%s:/UPM/%s", lv.Name, req.Name)
	config.HostConfig.Binds = append(config.HostConfig.Binds, name)

	return &lv, nil
}

func (sv sanVolume) Expand(lv database.Volume, agent string, size int64) error {
	lv, err := sv.iface.GetVolume(lv.Name)
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

	lv.Size += size

	lun, lv, err := sv.san.Extend(lv, size)
	if err != nil {
		return err
	}

	err = sv.san.Mapping(lv.EngineID, lv.VG, lun.ID)
	if err != nil {
		return err
	}

	err = expandSanVG(agent, sv.san.Vendor(), lun)
	if err != nil {
		return err
	}

	return updateVolume(agent, lv)
}

func (sv sanVolume) Recycle(lv database.Volume) error {
	err := sv.san.DelMapping(lv.Name)
	if errors.Cause(err) == sql.ErrNoRows {
		return nil
	}

	return sv.san.Recycle(lv.Name, 0)
}

func (sv sanVolume) createSanVG(vg string) error {
	list, err := sv.iface.ListLunByVG(vg)
	if err != nil {
		return err
	}

	addr := sv.engine.IP + ":" + sv.pluginPort

	return createSanVG(addr, sv.san.Vendor(), list)
}

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
	client, err := sdk.NewClient(addr, 30*time.Second, nil)
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
	client, err := sdk.NewClient(addr, 30*time.Second, nil)
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
	cli, err := sdk.NewClient(addr, 30*time.Second, nil)
	if err != nil {
		return err
	}

	return cli.VolumeUpdate(option)
}
