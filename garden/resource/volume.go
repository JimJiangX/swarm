package resource

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"

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
	Space() (space, error)
	Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error)

	Recycle(database.Volume) error
}

func newNFSDriver(no database.NodeOrmer, engineID string) (volumeDriver, error) {
	n, err := no.GetNode(engineID)
	if err != nil {
		return nil, err
	}

	sys, err := no.GetSysConfig()
	if err != nil {
		return nil, err
	}

	return NewNFSDriver(n.NFS, sys.BackupDir), nil
}

type _NFSDriver struct {
	database.NFS
	backupDir string
}

func NewNFSDriver(nfs database.NFS, backup string) _NFSDriver {
	return _NFSDriver{
		NFS:       nfs,
		backupDir: backup,
	}
}

func (nd _NFSDriver) Driver() string { return "NFS" }
func (nd _NFSDriver) Name() string   { return "" }
func (nd _NFSDriver) Type() string   { return "NFS" }

func (nd _NFSDriver) Space() (space, error) {
	out, err := execNFScmd(nd.Addr, nd.Dir, nd.MountDir, nd.Options)
	if err != nil {
		return space{}, err
	}

	total, free, err := parseNFSSpace(out)
	if err != nil {
		return space{}, err
	}

	return space{
		Total: total,
		Free:  free,
	}, nil
}

func (nd _NFSDriver) Alloc(config *cluster.ContainerConfig, uid string, req structs.VolumeRequire) (*database.Volume, error) {
	if req.Type == "NFS" || req.Type == "nfs" {
		config.HostConfig.Binds = append(config.HostConfig.Binds, nd.MountDir+":"+nd.backupDir)
	}
	return nil, nil
}

func (nd _NFSDriver) Recycle(database.Volume) error {

	return nil
}

func parseNFSSpace(in []byte) (int64, int64, error) {

	atoi := func(line, key []byte) (int64, error) {

		if i := bytes.Index(line, key); i != -1 {
			return strconv.ParseInt(string(bytes.TrimSpace(line[i+len(key):])), 10, 64)
		}

		return 0, errors.Errorf("key:%s not exist", key)
	}

	var total, free int64
	tkey := []byte("total_space:")
	fkey := []byte("free_space:")

	br := bufio.NewReader(bytes.NewReader(in))

	for {
		if total > 0 && free > 0 {
			return total, free, nil
		}

		line, _, err := br.ReadLine()
		if err != nil {
			if err == io.EOF {
				return total, free, nil
			}

			return total, free, errors.Wrapf(err, "parse nfs output error,input:'%s'", in)
		}

		n, err := atoi(line, tkey)
		if err == nil {
			total = n
			continue
		}

		n, err = atoi(line, fkey)
		if err == nil {
			free = n
		}
	}
}

func execNFScmd(ip, dir, mount, opts string) ([]byte, error) {
	const sh = "./script/nfs/get_NFS_space.sh"

	path, err := utils.GetAbsolutePath(false, sh)
	if err != nil {
		return nil, err
	}

	cmd, err := utils.ExecScript(path, ip, dir, mount, opts)
	if err != nil {
		return nil, err
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}

	return out, nil
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

func (lv localVolume) Space() (space, error) {
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

	name := fmt.Sprintf("%s:/DBAAS%s", v.Name, req.Name)
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

func (vds volumeDrivers) AllocVolumes(config *cluster.ContainerConfig, uid string, stores []structs.VolumeRequire) ([]*database.Volume, error) {
	volumes := make([]*database.Volume, 0, len(stores))

	for i := range stores {
		driver := vds.get(stores[i].Type)
		if driver == nil {
			return volumes, errors.New("not found the assigned volumeDriver:" + stores[i].Type)
		}

		v, err := driver.Alloc(config, uid, stores[i])
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

	vd, err := volumeDriverFromEngine(vo, e, _HDDVGLabel)
	if err == nil {
		drivers = append(drivers, vd)
	}

	vd, err = volumeDriverFromEngine(vo, e, _SSDVGLabel)
	if err == nil {
		drivers = append(drivers, vd)
	}

	return drivers, nil
}
