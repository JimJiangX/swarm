package storage

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

type huawei struct {
	ID       string `db:"id"`
	Vendor   string `db:"vendor"`
	Version  string `db:"version"`
	IPAddr   string `db:"ip_addr"`
	Username string `db:"username"`
	Password string `db:"password"`
	HluStart int    `db:"hlu_start"`
	HluEnd   int    `db:"hlu_end"`
}

type huaweiStore struct {
	lock   *sync.RWMutex
	script string

	orm database.StorageOrmer
	hs  huawei
}

func (h huaweiStore) scriptPath(file string) (string, error) {
	path, err := utils.GetAbsolutePath(false, h.script, file)
	if err != nil {
		return "", errors.Wrap(err, "not found file:"+file)
	}

	return path, nil
}

// NewHuaweiStore returns a new huawei store
func newHuaweiStore(orm database.StorageOrmer, script string, san database.SANStorage) Store {
	hw := huawei{
	// TODO:
	}
	return &huaweiStore{
		lock:   new(sync.RWMutex),
		orm:    orm,
		script: filepath.Join(script, hw.Vendor, hw.Version),
		hs:     hw,
	}
}

func (h huaweiStore) ID() string {
	return h.hs.ID
}

func (h huaweiStore) Vendor() string {
	return h.hs.Vendor
}

func (h huaweiStore) Driver() string {
	return SANStoreDriver
}

func (h *huaweiStore) ping() error {
	path, err := h.scriptPath("connect_test.sh")
	if err != nil {
		return err
	}

	logrus.Debug(path, h.hs.IPAddr, h.hs.Username, h.hs.Password)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.IPAddr, h.hs.Username, h.hs.Password)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *huaweiStore) insert() error {
	san := database.SANStorage{
	// TODO:
	}
	h.lock.Lock()
	err := h.orm.InsertSANStorage(san)
	h.lock.Unlock()

	return err
}

func (h *huaweiStore) Alloc(name, unit, vg string, size int64) (database.LUN, database.Volume, error) {
	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	lun := database.LUN{}
	lv := database.Volume{}

	out, err := h.Size()
	if err != nil {
		return lun, lv, err
	}
	for key := range out {
		if !key.Enabled {
			delete(out, key)
		}
	}

	rg := maxIdleSizeRG(out)
	if out[rg].Free < size {
		return lun, lv, errors.Errorf("%s hasn't enough space for alloction,max:%d < need:%d", h.Vendor(), out[rg].Free, size)
	}

	path, err := h.scriptPath("create_lun.sh")
	if err != nil {
		return lun, lv, err
	}
	// size:byte-->MB
	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password,
		rg.StorageRGID, name, strconv.Itoa(int(size)>>20 + 100)}

	logrus.Debug(param)

	cmd := utils.ExecScript(param...)
	output, err := cmd.Output()
	if err != nil {
		return lun, lv, errors.Errorf("exec:%s,Output:%s,%s", cmd.Args, output, err)
	}

	storageLunID, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" alloc LUN")
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            name,
		VG:              vg,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        int(size),
		StorageLunID:    storageLunID,
		CreatedAt:       time.Now(),
	}

	lv = database.Volume{
		ID:         utils.Generate64UUID(),
		Name:       name,
		Size:       size,
		UnitID:     unit,
		VG:         vg,
		Driver:     h.Driver(),
		DriverType: SANStore,
		Filesystem: DefaultFilesystemType,
	}

	err = h.orm.InsertLunVolume(lun, lv)
	if err != nil {
		return lun, lv, err
	}

	return lun, lv, nil
}

func (h *huaweiStore) Extend(lv database.Volume, size int64) (database.LUN, database.Volume, error) {
	lun := database.LUN{}

	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	out, err := h.Size()
	if err != nil {
		return lun, lv, err
	}
	for key := range out {
		if !key.Enabled {
			delete(out, key)
		}
	}

	rg := maxIdleSizeRG(out)
	if out[rg].Free < size {
		return lun, lv, errors.Errorf("%s hasn't enough space for alloction,max:%d < need:%d", h.Vendor(), out[rg].Free, size)
	}

	path, err := h.scriptPath("create_lun.sh")
	if err != nil {
		return lun, lv, err
	}
	// size:byte-->MB
	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password,
		rg.StorageRGID, lv.Name, strconv.Itoa(int(size)>>20 + 100)}

	logrus.Debug(param)

	cmd := utils.ExecScript(param...)
	output, err := cmd.Output()
	if err != nil {
		return lun, lv, errors.Errorf("exec:%s,Output:%s,%s", cmd.Args, output, err)
	}

	storageLunID, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" alloc LUN")
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            lv.Name,
		VG:              lv.VG,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        int(size),
		StorageLunID:    storageLunID,
		CreatedAt:       time.Now(),
	}

	lv.Size += size

	err = h.orm.InsertLunSetVolume(lun, lv)
	if err != nil {
		return lun, lv, err
	}

	return lun, lv, nil
}

func (h huaweiStore) ListLUN(nameOrVG string) ([]database.LUN, error) {
	return h.orm.ListLunByNameOrVG(nameOrVG)
}

func (h *huaweiStore) RecycleLUN(id string, lun int) error {
	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	var (
		l   database.LUN
		err error
	)
	if len(id) > 0 {
		l, err = h.orm.GetLUN(id)
	}
	if err != nil && lun > 0 {
		l, err = h.orm.GetLunByLunID(h.ID(), lun)
	}
	if err != nil {
		return err
	}

	path, err := h.scriptPath("del_lun.sh")
	if err != nil {
		return err
	}

	logrus.Debug(path, h.hs.IPAddr, h.hs.Username, h.hs.Password, l.StorageLunID)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(l.StorageLunID))
	if err != nil {
		return errors.WithStack(err)
	}

	err = h.orm.DelLUN(l.ID)

	return err
}

func (h huaweiStore) idleSize() (map[string]int64, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	rg, err := h.Size()
	if err != nil {
		return nil, err
	}

	out := make(map[string]int64, len(rg))
	for key, val := range rg {
		out[key.ID] = val.Free
	}

	return out, nil
}

func (h *huaweiStore) AddHost(name string, wwwn ...string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	path, err := h.scriptPath("add_host.sh")
	if err != nil {
		return err
	}

	if len(name) >= maxHostLen {
		name = name[:maxHostLen]
	}

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, name}

	logrus.Debug(param)

	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	_, err = utils.ExecContextTimeout(nil, 0, param...)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *huaweiStore) DelHost(name string, wwwn ...string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	path, err := h.scriptPath("del_host.sh")
	if err != nil {
		return err
	}

	if len(name) >= maxHostLen {
		name = name[:maxHostLen]
	}

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, name}

	logrus.Debug(param)

	h.lock.Lock()
	defer h.lock.Unlock()

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, param...)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *huaweiStore) Mapping(host, vg, lun, unit string) error {
	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	l, err := h.orm.GetLUN(lun)
	if err != nil {
		return err
	}
	lv, err := h.orm.GetVolume(l.Name)
	if err != nil {
		return err
	}

	out, err := h.orm.ListHostLunIDByMapping(host)
	if err != nil {
		return err
	}

	find, val := findIdleNum(h.hs.HluStart, h.hs.HluEnd, out)
	if !find {
		return errors.Errorf("%s:no available Host LUN ID", h.Vendor())
	}

	err = h.orm.LunMapping(lun, host, vg, val)
	if err != nil {
		return err
	}

	lv.EngineID = host
	lv.UnitID = unit

	err = h.orm.SetVolume(lv)
	if err != nil {
		return err
	}
	path, err := h.scriptPath("create_lunmap.sh")
	if err != nil {
		return err
	}

	if len(host) >= maxHostLen {
		host = host[:maxHostLen]
	}

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val)}

	logrus.Debug(param)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, param...)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *huaweiStore) DelMapping(lun database.LUN) error {

	path, err := h.scriptPath("del_lunmap.sh")
	if err != nil {
		return err
	}

	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(lun.StorageLunID)}

	logrus.Debug(param)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, param...)
	if err != nil {
		return errors.WithStack(err)
	}

	err = h.orm.DelLunMapping(lun.ID)

	return err
}

func (h *huaweiStore) AddSpace(id string) (Space, error) {
	_, err := h.orm.GetRaidGroup(h.ID(), id)
	if err == nil {
		return Space{}, errors.Errorf("RaidGroup %s is exist in %s", id, h.ID())
	}

	insert := func() error {
		rg := database.RaidGroup{
			ID:          utils.Generate32UUID(),
			StorageID:   h.ID(),
			StorageRGID: id,
			Enabled:     true,
		}

		return h.orm.InsertRaidGroup(rg)
	}

	// scan RaidGroup info
	h.lock.RLock()
	defer h.lock.RUnlock()

	spaces, err := h.list(id)
	if err != nil {
		return Space{}, err
	}

	for i := range spaces {
		if spaces[i].ID == id {

			if err = insert(); err == nil {
				return spaces[i], nil
			}

			return Space{}, err

		}
	}

	return Space{}, errors.Errorf("%s:Space %s is not exist", h.ID(), id)
}

func (h *huaweiStore) list(rg ...string) ([]Space, error) {
	list := ""
	if len(rg) == 0 {
		return nil, nil

	} else if len(rg) == 1 {
		list = rg[0]
	} else {
		list = strings.Join(rg, " ")
	}

	path, err := h.scriptPath("listrg.sh")
	if err != nil {
		return nil, err
	}

	logrus.Debug(path, h.hs.IPAddr, h.hs.Username, h.hs.Password, list)

	cmd := utils.ExecScript(path, h.hs.IPAddr, h.hs.Username, h.hs.Password, list)

	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, h.Vendor()+" list")
	}

	err = cmd.Start()
	if err != nil {
		return nil, errors.Errorf("Exec %s:%s", cmd.Args, err)
	}

	spaces := parseSpace(r)

	err = cmd.Wait()
	if err != nil {
		return nil, errors.Errorf("Wait %s:%s", cmd.Args, err)
	}

	if len(spaces) == 0 {
		return nil, nil
	}

	return spaces, nil
}

func (h *huaweiStore) EnableSpace(id string) error {
	h.lock.Lock()
	err := h.orm.SetRaidGroupStatus(h.ID(), id, true)
	h.lock.Unlock()

	return err
}

func (h *huaweiStore) DisableSpace(id string) error {
	h.lock.Lock()
	err := h.orm.SetRaidGroupStatus(h.ID(), id, false)
	h.lock.Unlock()

	return err
}

func (h *huaweiStore) removeSpace(id string) error {
	h.lock.Lock()
	err := h.orm.DelRaidGroup(h.ID(), id)
	h.lock.Unlock()

	return err
}

func (h huaweiStore) Size() (map[database.RaidGroup]Space, error) {
	out, err := h.orm.ListRGByStorageID(h.ID())
	if err != nil {
		return nil, err
	}

	rg := make([]string, len(out))

	for i, val := range out {
		rg[i] = val.StorageRGID
	}

	spaces, err := h.list(rg...)
	if err != nil {
		return nil, err
	}

	var info map[database.RaidGroup]Space

	if len(spaces) > 0 {
		info = make(map[database.RaidGroup]Space)

		for i := range out {
		loop:
			for s := range spaces {
				if out[i].StorageRGID == spaces[s].ID {
					spaces[s].Enable = out[i].Enabled
					info[out[i]] = spaces[s]
					break loop
				}
			}
		}
	}

	return info, nil
}

func (h huaweiStore) Info() (Info, error) {
	list, err := h.Size()
	if err != nil {
		return Info{}, err
	}
	info := Info{
		ID:     h.ID(),
		Vendor: h.Vendor(),
		Driver: h.Driver(),
		Fstype: DefaultFilesystemType,
		List:   make(map[string]Space, len(list)),
	}

	for rg, val := range list {
		info.List[rg.StorageRGID] = val
		info.Total += val.Total
		info.Free += val.Free
	}

	return info, nil
}