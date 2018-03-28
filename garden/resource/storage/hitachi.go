package storage

import (
	"fmt"
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

type hitachi struct {
	ID        string `db:"id"`
	Vendor    string `db:"vendor"`
	Version   string `db:"version"`
	AdminUnit string `db:"admin_unit"`
	LunStart  int    `db:"lun_start"`
	LunEnd    int    `db:"lun_end"`
	HluStart  int    `db:"hlu_start"`
	HluEnd    int    `db:"hlu_end"`
}

// hitachi store
type hitachiStore struct {
	lock   *sync.RWMutex
	script string

	orm database.StorageOrmer
	hs  hitachi

	rgList map[string]Space // cache rg spaces
}

// NewHitachiStore returns a new Store
func newHitachiStore(orm database.StorageOrmer, script string, san database.SANStorage) Store {
	hs := hitachi{
		ID:        san.ID,
		Vendor:    san.Vendor,
		Version:   san.Version,
		AdminUnit: san.AdminUnit,
		LunStart:  san.LunStart,
		LunEnd:    san.LunEnd,
		HluStart:  san.HluStart,
		HluEnd:    san.HluEnd,
	}

	return &hitachiStore{
		lock:   new(sync.RWMutex),
		orm:    orm,
		script: filepath.Join(script, hs.Vendor, hs.Version),
		hs:     hs,
		rgList: make(map[string]Space),
	}
}

func (h hitachiStore) scriptPath(file string) (string, error) {
	path, err := utils.GetAbsolutePath(false, h.script, file)
	if err != nil {
		return "", errors.Wrap(err, "not found file:"+file)
	}

	return path, nil
}

// ID returns store ID
func (h hitachiStore) ID() string {
	return h.hs.ID
}

// Vendor returns store vendor
func (h hitachiStore) Vendor() string {
	return h.hs.Vendor
}

// Driver returns store driver
func (h hitachiStore) Driver() string {
	return SANStoreDriver
}

// ping connected to store,call connect_test.sh,test whether store available.
func (h hitachiStore) ping() error {
	path, err := h.scriptPath("connect_test.sh")
	if err != nil {
		return err
	}

	logrus.Debugf("%s %s", path, h.hs.AdminUnit)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit)

	return errors.WithStack(err)
}

// insert insert hitachiStore into DB
func (h *hitachiStore) insert() error {
	san := database.SANStorage{
		ID:        h.hs.ID,
		Vendor:    h.hs.Vendor,
		Version:   h.hs.Version,
		AdminUnit: h.hs.AdminUnit,
		LunStart:  h.hs.LunStart,
		LunEnd:    h.hs.LunEnd,
		HluStart:  h.hs.HluStart,
		HluEnd:    h.hs.HluEnd,
	}
	h.lock.Lock()
	err := h.orm.InsertSANStorage(san)
	h.lock.Unlock()

	return err
}

// Alloc list hitachiStore's RG idle space,alloc a new LUN in free space,
// the allocated LUN is used to creating a volume.
// alloction calls create_lun.sh
func (h *hitachiStore) Alloc(name, unit, vg, host string, size int64) (database.LUN, database.Volume, error) {
	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	lun := database.LUN{}
	lv := database.Volume{}

	out, err := h.Size()
	if err != nil {
		return lun, lv, err
	}

	rg := maxIdleSizeRG(out)
	if out[rg].Free < size {
		return lun, lv, errors.Errorf("%s hasn't enough space for alloction,max:%d < need:%d", h.Vendor(), out[rg].Free, size)
	}

	used, err := h.orm.ListLunIDBySystemID(h.ID())
	if err != nil {
		return lun, lv, err
	}

	ok, id := findIdleNum(h.hs.LunStart, h.hs.LunEnd, used)
	if !ok {
		return lun, lv, errors.New("no available LUN ID in store:" + h.Vendor())
	}

	hluns, err := h.orm.ListHostLunIDByMapping(host)
	if err != nil {
		return lun, lv, err
	}

	find, val := findIdleNum(h.hs.HluStart, h.hs.HluEnd, hluns)
	if !find {
		return lun, lv, errors.Errorf("%s:no available host LUN ID", h.Vendor())
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            name,
		VG:              vg,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        int(size),
		StorageLunID:    id,
		MappingTo:       host,
		HostLunID:       val,
		CreatedAt:       time.Now(),
	}

	lv = database.Volume{
		ID:         utils.Generate64UUID(),
		Name:       name,
		UnitID:     unit,
		EngineID:   host,
		Size:       size,
		VG:         vg,
		Driver:     h.Driver(),
		DriverType: SANStore,
		Filesystem: DefaultFilesystemType,
	}

	err = h.orm.InsertLunVolume(lun, lv)
	if err != nil {
		return lun, lv, err
	}

	defer func() {
		if err == nil {
			return
		}

		lun.MappingTo = ""
	}()

	path, err := h.scriptPath("create_lun.sh")
	if err != nil {
		return lun, lv, err
	}
	// size:byte-->MB
	param := []string{path, h.hs.AdminUnit,
		rg.StorageRGID, strconv.Itoa(id), strconv.Itoa(int(size)>>20 + 100)}

	logrus.Debug(param)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, param...)
	if err != nil {
		return lun, lv, errors.WithStack(err)
	}

	{
		space := out[rg]
		space.Free -= size
		h.updateRGCache(space)
	}

	path, err = h.scriptPath("create_lunmap.sh")
	if err != nil {
		return lun, lv, err
	}

	host = generateHostName(host)

	logrus.Debugf("%s %s %d %s %d", path, h.hs.AdminUnit, lun.StorageLunID, host, val)

	time.Sleep(time.Second)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit,
		strconv.Itoa(lun.StorageLunID), host, strconv.Itoa(val))
	if err != nil {
		return lun, lv, errors.WithStack(err)
	}

	return lun, lv, nil
}

func (h *hitachiStore) Extend(lv database.Volume, size int64) (lun database.LUN, v database.Volume, err error) {
	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	out, err := h.Size()
	if err != nil {
		return lun, lv, err
	}

	rg := maxIdleSizeRG(out)
	if out[rg].Free < size {
		return lun, lv, errors.Errorf("%s hasn't enough space for alloction,max:%d < need:%d", h.Vendor(), out[rg].Free, size)
	}

	used, err := h.orm.ListLunIDBySystemID(h.ID())
	if err != nil {
		return lun, lv, err
	}

	ok, id := findIdleNum(h.hs.LunStart, h.hs.LunEnd, used)
	if !ok {
		return lun, lv, errors.New("no available LUN ID in store:" + h.Vendor())
	}

	hluns, err := h.orm.ListHostLunIDByMapping(lv.EngineID)
	if err != nil {
		return lun, lv, err
	}

	find, val := findIdleNum(h.hs.HluStart, h.hs.HluEnd, hluns)
	if !find {
		return lun, lv, errors.Errorf("%s:no available host LUN ID", h.Vendor())
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            lv.Name,
		VG:              lv.VG,
		MappingTo:       lv.EngineID,
		HostLunID:       val,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        int(size),
		StorageLunID:    id,
		CreatedAt:       time.Now(),
	}

	lv.Size += size

	err = h.orm.InsertLunSetVolume(lun, lv)
	if err != nil {
		return lun, lv, err
	}

	keepLun := false

	defer func() {
		if err == nil {
			return
		}

		if !keepLun {
			_err := h.orm.DelLUN(lun.ID)
			if _err != nil {
				err = fmt.Errorf("expand lun failed,DelLUN failed,%+v\n%+v", _err, err)
			}
		}

		lun.MappingTo = ""
		lv.Size -= size

		_err := h.orm.SetVolume(lv)
		if _err != nil {
			lv.Size += size
			err = fmt.Errorf("expand lun failed,SetVolume failed,%+v\n%+v", _err, err)
		}
	}()

	// create lun
	path, err := h.scriptPath("create_lun.sh")
	if err != nil {
		return lun, lv, err
	}
	// size:byte-->MB
	param := []string{path, h.hs.AdminUnit,
		rg.StorageRGID, strconv.Itoa(id), strconv.Itoa(int(size)>>20 + 100)}

	logrus.Debug(param)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, param...)
	if err != nil {
		return lun, lv, errors.WithStack(err)
	}

	keepLun = true

	defer func() {
		if err == nil {
			return
		}

		h.invalidRGCache(lun.RaidGroupID)

		path, _err := h.scriptPath("del_lun.sh")
		if _err != nil {
			err = fmt.Errorf("%s\n%+v", _err, err)
			return
		}

		logrus.Debugf("%s %s %d", path, h.hs.AdminUnit, lun.StorageLunID)
		time.Sleep(time.Second)

		_, _err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit, strconv.Itoa(lun.StorageLunID))
		if _err != nil {
			err = fmt.Errorf("%s\n%+v", _err, err)
		} else {
			keepLun = false
			lun.MappingTo = ""
		}
	}()

	// lun mapping
	path, err = h.scriptPath("create_lunmap.sh")
	if err != nil {
		return lun, lv, err
	}

	host := generateHostName(lv.EngineID)

	logrus.Debugf("%s %s %d %s %d", path, h.hs.AdminUnit, lun.StorageLunID, host, val)

	defer func() {
		if err == nil {
			return
		}

		path, _err := h.scriptPath("del_lunmap.sh")
		if _err != nil {
			err = fmt.Errorf("%s\n%+v", _err, err)
			return
		}

		logrus.Debugf("%s %s %d", path, h.hs.AdminUnit, lun.StorageLunID)

		time.Sleep(time.Second)

		_, _err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit, strconv.Itoa(lun.StorageLunID))
		if _err != nil {
			err = fmt.Errorf("%s\n%+v", _err, err)
			keepLun = true
		}
	}()

	time.Sleep(time.Second)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit,
		strconv.Itoa(lun.StorageLunID), host, strconv.Itoa(val))
	if err != nil {
		return lun, lv, errors.WithStack(err)
	}

	{
		space := out[rg]
		space.Free -= size
		h.updateRGCache(space)
	}

	return lun, lv, nil
}

func (h hitachiStore) ListLUN(nameOrVG string) ([]database.LUN, error) {
	return h.orm.ListLunByNameVG(nameOrVG)
}

// Recycle calls del_lun.sh,make the lun available for alloction.
func (h *hitachiStore) RecycleLUN(id string, lun int) error {
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

	h.invalidRGCache(l.RaidGroupID)

	path, err := h.scriptPath("del_lun.sh")
	if err != nil {
		return err
	}

	logrus.Debugf("%s %s %d", path, h.hs.AdminUnit, l.StorageLunID)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit, strconv.Itoa(l.StorageLunID))
	if err != nil {
		return errors.WithStack(err)
	}

	return h.orm.DelLUN(l.ID)
}

// Size list store's RGs infomation
func (h hitachiStore) Size() (map[database.RaidGroup]Space, error) {
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
			space := spaces[out[i].StorageRGID]
			space.Enable = out[i].Enabled
			info[out[i]] = space
		}
	}

	return info, nil
}

// list store RGs info,calls listrg.sh
func (h *hitachiStore) list(rg ...string) (map[string]Space, error) {
	if len(rg) > 0 {
		// find hitachiStore rg list in rgList cache
		spaces := h.getRGSpacesFromCache(rg...)
		if spaces != nil {
			return spaces, nil
		}
	}

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

	logrus.Debugf("%s %s %s", path, h.hs.AdminUnit, list)

	cmd := utils.ExecScript(path, h.hs.AdminUnit, list)

	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, h.Vendor()+" list")
	}

	err = cmd.Start()
	if err != nil {
		return nil, errors.Errorf("Exec %s:%s", cmd.Args, err)
	}

	spaces, warnings := parseSpace(r)
	if len(warnings) > 0 {
		logrus.Warningf("parse SAN RG warinings:%s", warnings)
	}

	err = cmd.Wait()
	if err != nil {
		return nil, errors.Errorf("Wait %s:%s,warnings:%s", cmd.Args, err, warnings)
	}

	// cache hitachiStore rg list spaces
	if len(spaces) == 0 {
		h.invalidRGCache(rg...)
	} else {
		h.updateRGListCache(spaces)
	}

	return spaces, nil
}

// idleSize list store's RGs free size.
func (h hitachiStore) idleSize() (map[string]int64, error) {
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

// AddHost register a host to store,calls add_host.sh,
// the host able to connect with store and shared store space.
func (h *hitachiStore) AddHost(name string, wwwn ...string) error {
	path, err := h.scriptPath("add_host.sh")
	if err != nil {
		return err
	}

	name = generateHostName(name)

	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}
	param = append(param, wwwn...)

	logrus.Debug(param)

	_, err = utils.ExecContextTimeout(nil, 0, param...)

	return errors.WithStack(err)
}

// DelHost deregister host,calls del_host.sh
func (h *hitachiStore) DelHost(name string, wwwn ...string) error {
	path, err := h.scriptPath("del_host.sh")
	if err != nil {
		return err
	}

	name = generateHostName(name)

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}

	logrus.Debug(param)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, param...)

	return errors.WithStack(err)
}

// Mapping calls create_lunmap.sh,associate LUN with host.
func (h *hitachiStore) Mapping(host, vg, lun, unit string) (database.LUN, error) {
	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	l, err := h.orm.GetLUN(lun)
	if err != nil {
		return l, err
	}
	lv, err := h.orm.GetVolume(l.Name)
	if err != nil {
		return l, err
	}

	out, err := h.orm.ListHostLunIDByMapping(host)
	if err != nil {
		return l, err
	}

	find, val := findIdleNum(h.hs.HluStart, h.hs.HluEnd, out)
	if !find {
		return l, errors.Errorf("%s:no available host LUN ID", h.Vendor())
	}

	err = h.orm.LunMapping(lun, host, vg, val)
	if err != nil {
		return l, err
	}

	lv.EngineID = host
	lv.UnitID = unit

	err = h.orm.SetVolume(lv)
	if err != nil {
		return l, err
	}

	path, err := h.scriptPath("create_lunmap.sh")
	if err != nil {
		return l, err
	}

	host = generateHostName(host)

	logrus.Debugf("%s %s %d %s %d", path, h.hs.AdminUnit, l.StorageLunID, host, val)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit,
		strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val))
	if err != nil {
		return l, errors.WithStack(err)
	}

	l.MappingTo = host
	l.VG = vg
	l.HostLunID = val

	return l, nil
}

// DelMapping disassociate of the lun from host,calls del_lunmap.sh
func (h *hitachiStore) DelMapping(lun database.LUN) error {
	path, err := h.scriptPath("del_lunmap.sh")
	if err != nil {
		return err
	}

	time.Sleep(time.Second)
	h.lock.Lock()
	defer h.lock.Unlock()

	logrus.Debugf("%s %s %d", path, h.hs.AdminUnit, lun.StorageLunID)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, path, h.hs.AdminUnit,
		strconv.Itoa(lun.StorageLunID))
	if err != nil {
		return errors.WithStack(err)
	}

	return h.orm.DelLunMapping(lun.ID)
}

// AddSpace add a new RG already existed in the store,
func (h *hitachiStore) AddSpace(id string) (Space, error) {
	_, err := h.orm.GetRaidGroup(h.ID(), id)
	if err == nil {
		return Space{}, err
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

	h.invalidRGCache(id)

	spaces, err := h.list(id)
	if err != nil {
		return Space{}, err
	}

	if val, ok := spaces[id]; ok {
		return val, insert()
	}

	return Space{}, errors.Errorf("%s:Space %s is not exist", h.ID(), id)
}

// EnableSpace signed RG enabled
func (h *hitachiStore) EnableSpace(id string) error {
	h.lock.Lock()

	h.invalidRGCache(id)
	err := h.orm.SetRaidGroupStatus(h.ID(), id, true)

	h.lock.Unlock()

	return err
}

// DisableSpace signed RG disabled
func (h *hitachiStore) DisableSpace(id string) error {
	h.lock.Lock()

	h.invalidRGCache(id)
	err := h.orm.SetRaidGroupStatus(h.ID(), id, false)

	h.lock.Unlock()

	return err
}

func (h *hitachiStore) removeSpace(id string) error {
	h.lock.Lock()

	h.invalidRGCache(id)
	err := h.orm.DelRaidGroup(h.ID(), id)

	h.lock.Unlock()

	return err
}

// Info returns store infomation
func (h hitachiStore) Info() (Info, error) {
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

		if rg.Enabled {
			info.Free += val.Free
		}
	}

	return info, nil
}

func (h hitachiStore) getRGSpacesFromCache(rg ...string) map[string]Space {
	spaces := make(map[string]Space, len(rg))

	for i := range rg {
		val, ok := h.rgList[rg[i]]

		if !ok || val.ID == "" {
			return nil
		}

		spaces[rg[i]] = val
	}

	return spaces
}

func (h *hitachiStore) invalidRGCache(rg ...string) {
	for i := range rg {
		delete(h.rgList, rg[i])
	}
}

func (h *hitachiStore) updateRGCache(space Space) {
	h.rgList[space.ID] = space
}

func (h *hitachiStore) updateRGListCache(spaces map[string]Space) {
	for id, val := range spaces {
		if id == val.ID {
			h.rgList[id] = val
		}
	}
}
