package driver

import (
	"fmt"
	"strconv"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/seed/sdk"
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
	san, err := stores.GetStore(storeID)
	if err != nil {
		return nil, err
	}

	return &sanVolume{
		iface:      iface,
		san:        san,
		engine:     e,
		pluginPort: strconv.Itoa(sys.Plugin),
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

	lun, v, err := sv.san.Alloc(name, uid, vg, int(req.Size))
	if err != nil {
		return nil, err
	}

	err = sv.san.Mapping(sv.engine.ID, vg, lun.ID)
	if err != nil {
		return &v, err
	}

	err = sv.createSanVG(vg)
	if err != nil {
		return &v, err
	}

	name = fmt.Sprintf("%s:/UPM/%s", v.Name, req.Name)
	config.HostConfig.Binds = append(config.HostConfig.Binds, name)

	return &v, nil
}

func (sv sanVolume) Recycle(lv database.Volume) error {

	return sv.san.Recycle(lv.Name, 0)
}

func (sv sanVolume) createSanVG(vg string) error {
	//	fmt.Printf("Engine %s create San Storeage VG,VG=%s\n", host, vg)

	list, err := sv.iface.ListLunByVG(vg)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return nil
	}

	l, size := make([]int, len(list)), 0

	for i := range list {
		l[i] = list[i].HostLunID
		size += list[i].SizeByte
	}

	config := sdk.VgConfig{
		HostLunID: l,
		VgName:    list[0].VG,
		Type:      sv.san.Vendor(),
	}

	addr := sv.engine.IP + ":" + sv.pluginPort
	client, err := sdk.NewClient(addr, 0, nil)
	if err != nil {
		return err
	}

	return client.SanVgCreate(config)
}
