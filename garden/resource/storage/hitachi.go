package storage

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

// hitachi store
type hitachiStore struct {
	lock   *sync.RWMutex
	script string

	orm database.StorageOrmer
	hs  database.HitachiStorage
}

// NewHitachiStore returns a new Store
func newHitachiStore(orm database.StorageOrmer, script string, hs database.HitachiStorage) Store {
	return &hitachiStore{
		lock:   new(sync.RWMutex),
		orm:    orm,
		script: filepath.Join(script, HITACHI, hs.Version),
		hs:     hs,
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

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, path, h.hs.AdminUnit)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// insert insert hitachiStore into DB
func (h *hitachiStore) insert() error {
	h.lock.Lock()
	err := h.orm.InsertHitachiStorage(h.hs)
	h.lock.Unlock()

	return err
}

// Alloc list hitachiStore's RG idle space,alloc a new LUN in free space,
// the allocated LUN is used to creating a volume.
// alloction calls create_lun.sh
func (h *hitachiStore) Alloc(name, unit, vg string, size int64) (database.LUN, database.Volume, error) {
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

	used, err := h.orm.ListLunIDBySystemID(h.ID())
	if err != nil {
		return lun, lv, err
	}

	ok, id := findIdleNum(h.hs.LunStart, h.hs.LunEnd, used)
	if !ok {
		return lun, lv, errors.New("no available LUN ID in store:" + h.Vendor())
	}

	path, err := h.scriptPath("create_lun.sh")
	if err != nil {
		return lun, lv, err
	}
	// size:byte-->MB
	param := []string{path, h.hs.AdminUnit,
		rg.StorageRGID, strconv.Itoa(id), strconv.Itoa(int(size)>>20 + 100)}

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, param...)
	if err != nil {
		return lun, lv, errors.WithStack(err)
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            name,
		VG:              vg,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        int(size),
		StorageLunID:    id,
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

func (h *hitachiStore) Extend(lv database.Volume, size int64) (database.LUN, database.Volume, error) {
	lun := database.LUN{}

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

	used, err := h.orm.ListLunIDBySystemID(h.ID())
	if err != nil {
		return lun, lv, err
	}

	ok, id := findIdleNum(h.hs.LunStart, h.hs.LunEnd, used)
	if !ok {
		return lun, lv, errors.New("no available LUN ID in store:" + h.Vendor())
	}

	path, err := h.scriptPath("create_lun.sh")
	if err != nil {
		return lun, lv, err
	}
	// size:byte-->MB
	param := []string{path, h.hs.AdminUnit,
		rg.StorageRGID, strconv.Itoa(id), strconv.Itoa(int(size)>>20 + 100)}

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, param...)
	if err != nil {
		return lun, lv, errors.WithStack(err)
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            lv.Name,
		VG:              lv.VG,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        int(size),
		StorageLunID:    id,
		CreatedAt:       time.Now(),
	}

	lv.Size += size

	err = h.orm.InsertLunSetVolume(lun, lv)

	return lun, lv, err
}

func (h hitachiStore) ListLUN(nameOrVG string) ([]database.LUN, error) {
	return h.orm.ListLunByNameOrVG(nameOrVG)
}

// Recycle calls del_lun.sh,make the lun available for alloction.
func (h *hitachiStore) Recycle(id string, lun int) error {
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

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, path, h.hs.AdminUnit, strconv.Itoa(l.StorageLunID))
	if err != nil {
		return errors.WithStack(err)
	}

	err = h.orm.DelLUN(l.ID)

	return err
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

// list store RGs info,calls listrg.sh
func (h *hitachiStore) list(rg ...string) ([]Space, error) {
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

	cmd := utils.ExecScript(path, h.hs.AdminUnit, list)

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

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}
	param = append(param, wwwn...)

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, param...)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// DelHost deregister host,calls del_host.sh
func (h *hitachiStore) DelHost(name string, wwwn ...string) error {
	path, err := h.scriptPath("del_host.sh")
	if err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, param...)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// Mapping calls create_lunmap.sh,associate LUN with host.
func (h *hitachiStore) Mapping(host, vg, lun, unit string) error {
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
		return errors.Errorf("%s:no available host LUN ID", h.Vendor())
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

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, path, h.hs.AdminUnit,
		strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val))
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// DelMapping disassociate of the lun from host,calls del_lunmap.sh
func (h *hitachiStore) DelMapping(lun database.LUN) error {
	path, err := h.scriptPath("del_lunmap.sh")
	if err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	_, err = utils.ExecContextTimeout(nil, defaultTimeout, debug, path, h.hs.AdminUnit,
		strconv.Itoa(lun.StorageLunID))
	if err != nil {
		return errors.WithStack(err)
	}

	err = h.orm.DelLunMapping(lun.ID)

	return err
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

// EnableSpace signed RG enabled
func (h *hitachiStore) EnableSpace(id string) error {
	h.lock.Lock()
	err := h.orm.SetRaidGroupStatus(h.ID(), id, true)
	h.lock.Unlock()

	return err
}

// DisableSpace signed RG disabled
func (h *hitachiStore) DisableSpace(id string) error {
	h.lock.Lock()
	err := h.orm.SetRaidGroupStatus(h.ID(), id, false)
	h.lock.Unlock()

	return err
}

func (h *hitachiStore) removeSpace(id string) error {
	h.lock.Lock()
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
		info.Free += val.Free
	}

	return info, nil
}
