package driver

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

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

type localVolumeIface interface {
	InsertVolume(lv database.Volume) error

	SetVolume(database.Volume) error

	GetVolume(nameOrID string) (database.Volume, error)

	ListVolumeByVG(string) ([]database.Volume, error)

	DelVolume(nameOrID string) error
}

type localVolumeMap struct {
	m map[string]database.Volume
}

func (lvm *localVolumeMap) len() int {
	return len(lvm.m)
}

func (lvm *localVolumeMap) InsertVolume(lv database.Volume) error {
	if lv.ID == "" {
		return errors.New("ID is required")
	}

	if lvm == nil {
		lvm = &localVolumeMap{make(map[string]database.Volume)}
	}

	if lvm.m == nil {
		lvm.m = make(map[string]database.Volume)
	}

	if _, ok := lvm.m[lv.ID]; ok {
		return errors.New("Volume ID existed")
	}

	for _, v := range lvm.m {
		if v.Name == lv.Name {
			return errors.New("Volume Name existed")
		}
	}

	lvm.m[lv.ID] = lv

	return nil
}

func (lvm *localVolumeMap) SetVolume(v database.Volume) error {
	lv, err := lvm.GetVolume(v.ID)
	if err != nil {
		return err
	}

	lvm.m[lv.ID] = lv

	return nil
}

func (lvm localVolumeMap) GetVolume(nameOrID string) (database.Volume, error) {

	if v, ok := lvm.m[nameOrID]; ok {
		return v, nil
	}

	for _, v := range lvm.m {
		if v.Name == nameOrID {
			return v, nil
		}
	}

	return database.Volume{}, sql.ErrNoRows
}

func (lvm localVolumeMap) ListVolumeByVG(vg string) ([]database.Volume, error) {
	out := make([]database.Volume, 0, 5)

	for _, v := range lvm.m {
		if v.VG == vg {
			out = append(out, v)
		}
	}

	return out, nil
}

func (lvm *localVolumeMap) DelVolume(nameOrID string) error {
	delete(lvm.m, nameOrID)

	for _, v := range lvm.m {
		if v.Name == nameOrID {
			delete(lvm.m, v.ID)
		}
	}

	return nil
}

func parseSize(size string) (int64, error) {
	siz := strings.TrimSpace(size)

	if len(siz) == 0 {
		return 0, nil
	}

	var bits uint

	switch siz[len(siz)-1] {
	case 'b', 'B':
		siz = strings.TrimSpace(siz[:len(siz)-1])
		bits = 0

	case 'k', 'K':
		siz = strings.TrimSpace(siz[:len(siz)-1])
		bits = 10

	case 'm', 'M':
		siz = strings.TrimSpace(siz[:len(siz)-1])
		bits = 20

	case 'g', 'G':
		siz = strings.TrimSpace(siz[:len(siz)-1])
		bits = 30
	}

	n, err := strconv.ParseInt(siz, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "ParseInt '%s' error", size)
	}

	return n << bits, nil
}

func volumeDriverFromEngine(iface localVolumeIface, e *cluster.Engine, label string, port int) (Driver, error) {
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
		t, err := parseSize(size)
		if err != nil {
			return &localVolume{}, err
		}
		total = t
	}

	lv := &localVolume{
		engine: e,
		vo:     iface,
		_type:  vgType,
		port:   port,
		driver: defaultLocalVolumeDriver,
		space: Space{
			Total:  total,
			Free:   total,
			VG:     vg,
			Fstype: defaultFileSystem,
		},
		vgIface: unsupportSAN{},
	}

	_, err := lv.Space()

	return lv, err
}

func localVolumeDrivers(e *cluster.Engine, iface localVolumeIface, port int) ([]Driver, error) {
	drivers := make([]Driver, 0, 4)

	vd, err := volumeDriverFromEngine(iface, e, _HDDVGLabel, port)
	if err == nil && vd != nil {
		drivers = append(drivers, vd)
	} else {
		logrus.Debugf("%s %s %+v", e.Name, _HDDVGLabel, err)
	}

	vd, err = volumeDriverFromEngine(iface, e, _SSDVGLabel, port)
	if err == nil && vd != nil {
		drivers = append(drivers, vd)
	} else {
		logrus.Debugf("%s %s %+v", e.Name, _SSDVGLabel, err)
	}

	return drivers, nil
}

type localVolume struct {
	vgIface
	engine *cluster.Engine
	driver string
	_type  string
	port   int
	space  Space
	vo     localVolumeIface
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
	lvs, err := lv.vo.ListVolumeByVG(lv.space.VG)
	if err != nil {
		return Space{}, err
	}

	var used int64
	for i := range lvs {
		used += lvs[i].Size
	}

	lv.space.Free = lv.space.Total - used

	return lv.space, nil
}

func setVolumeBind(config *cluster.ContainerConfig, lvName, req string) {
	name := fmt.Sprintf("%s:/DBAAS%s", lvName, req)
	config.HostConfig.Binds = append(config.HostConfig.Binds, name)

	if config.Config.Env == nil {
		config.Config.Env = make([]string, 0, 3)
	}
	config.Config.Env = append(config.Config.Env, req+"_DIR=/DBAAS"+req)
}

func (lv *localVolume) Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error) {
	space, err := lv.Space()
	if err != nil {
		return nil, err
	}

	if space.Free < req.Size {
		return nil, errors.Errorf("node %s local volume driver has no enough space:%d<%d", lv.engine.IP, space.Free, req.Size)
	}

	tag := config.Config.Labels["service.tag"]

	v := database.Volume{
		Size:       req.Size,
		ID:         utils.Generate32UUID(),
		Name:       strings.Join([]string{uid[:8], tag, req.Name}, "_"),
		UnitID:     uid,
		EngineID:   lv.engine.ID,
		VG:         space.VG,
		Driver:     lv.Driver(),
		DriverType: lv.Type(),
		Filesystem: space.Fstype,
	}

	err = lv.vo.InsertVolume(v)
	if err != nil {
		return nil, err
	}

	lv.space.Free -= req.Size

	setVolumeBind(config, v.Name, req.Name)

	return &v, nil
}

func (lv *localVolume) Expand(dv database.Volume, size int64) error {
	if size <= 0 {
		return nil
	}

	dv, err := lv.vo.GetVolume(dv.Name)
	if err != nil {
		return err
	}

	space, err := lv.Space()
	if err != nil {
		return err
	}

	if space.Free < size {
		return errors.Errorf("node %s local volume driver has no enough space for expansion:%d<%d", lv.engine.IP, space.Free, size)
	}

	dv.Size += size

	err = lv.vo.SetVolume(dv)
	if err != nil {
		return err
	}

	lv.space.Free -= size

	agent := fmt.Sprintf("%s:%d", lv.engine.IP, lv.port)

	return updateVolume(agent, dv)
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
