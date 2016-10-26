package storage

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
)

// hitachi store
type hitachiStore struct {
	lock *sync.RWMutex
	hs   database.HitachiStorage
}

// NewHitachiStore returns a new Store
func NewHitachiStore(vendor, admin string, lstart, lend, hstart, hend int) Store {
	return &hitachiStore{
		lock: new(sync.RWMutex),
		hs: database.HitachiStorage{
			ID:        utils.Generate64UUID(),
			Vendor:    vendor,
			AdminUnit: admin,
			LunStart:  lstart,
			LunEnd:    lend,
			HluStart:  hstart,
			HluEnd:    hend,
		},
	}
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

// Ping connected to store,call connect_test.sh,test whether store available.
func (h hitachiStore) Ping() error {
	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "connect_test.sh")
	if err != nil {
		return errors.Wrap(err, "ping hitachi store:"+h.Vendor())
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit)
	if err != nil {
		return errors.Wrap(err, "ping hitachi store:"+h.Vendor())
	}

	output, err := cmd.Output()
	fmt.Printf("exec:%s %s\n%s,error=%v\n", cmd.Path, cmd.Args, output, err)
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	return nil
}

// Insert insert hitachiStore into DB
func (h *hitachiStore) Insert() error {
	h.lock.Lock()
	err := h.hs.Insert()
	h.lock.Unlock()

	if err != nil {
		return errors.Wrap(err, "insert hitachi store")
	}

	return nil
}

// Alloc list hitachiStore's RG idle space,alloc a new LUN in free space,
// the allocated LUN is used to creating a volume.
// alloction calls create_lun.sh
func (h *hitachiStore) Alloc(name, unit, vg string, size int) (database.LUN, database.LocalVolume, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	lun := database.LUN{}
	lv := database.LocalVolume{}

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

	used, err := database.ListLunIDBySystemID(h.ID())
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" store alloc")
	}

	ok, id := findIdleNum(h.hs.LunStart, h.hs.LunEnd, used)
	if !ok {
		return lun, lv, errors.New("no available LUN ID in store:" + h.Vendor())
	}

	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "create_lun.sh")
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" alloc LUN")
	}
	// size:byte-->MB
	param := []string{path, h.hs.AdminUnit,
		strconv.Itoa(rg.StorageRGID), strconv.Itoa(id), strconv.Itoa(size>>20 + 100)}

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" alloc LUN")
	}
	output, err := cmd.Output()
	fmt.Printf("exec:%s %s\n%s,error=%v\n", cmd.Path, cmd.Args, output, err)
	if err != nil {
		return lun, lv, errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            name,
		VGName:          vg,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        size,
		StorageLunID:    id,
		CreatedAt:       time.Now(),
	}

	lv = database.LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       name,
		Size:       size,
		UnitID:     unit,
		VGName:     vg,
		Driver:     h.Driver(),
		Filesystem: DefaultFilesystemType,
	}

	err = database.TxInsertLUNAndVolume(lun, lv)
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" alloc LUN")
	}

	return lun, lv, nil
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
		l, err = database.GetLUNByID(id)
	}
	if err != nil && lun > 0 {
		l, err = database.GetLUNByLunID(h.ID(), lun)
	}
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" recycle")
	}

	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "del_lun.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" recycle")
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit, strconv.Itoa(l.StorageLunID))
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" recycle LUN")
	}

	output, err := cmd.Output()
	fmt.Printf("exec:%s %s\n%s,error=%v\n", cmd.Path, cmd.Args, output, err)
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	err = database.DelLUN(l.ID)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" recycle")
	}

	return nil
}

// Size list store's RGs infomation
func (h hitachiStore) Size() (map[database.RaidGroup]Space, error) {
	out, err := database.ListRGByStorageID(h.ID())
	fmt.Println("san:", h.ID(), out, err)
	if err != nil {
		return nil, errors.Wrap(err, h.Vendor()+" size")
	}

	rg := make([]int, len(out))

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
func (h *hitachiStore) list(rg ...int) ([]Space, error) {
	list := ""
	if len(rg) == 0 {
		return nil, nil

	} else if len(rg) == 1 {
		list = strconv.Itoa(rg[0])
	} else {
		list = intSliceToString(rg, " ")
	}

	fmt.Println("RG list:", list)

	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "listrg.sh")
	if err != nil {
		return nil, errors.Wrap(err, h.Vendor()+" list")
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit, list)
	if err != nil {
		return nil, errors.Wrap(err, h.Vendor()+" list")
	}

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

// IdleSize list store's RGs free size.
func (h hitachiStore) IdleSize() (map[string]int, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	rg, err := h.Size()
	if err != nil {
		return nil, errors.Wrap(err, h.Vendor()+" idle size")
	}

	out := make(map[string]int, len(rg))
	for key, val := range rg {
		out[key.ID] = val.Free
	}

	return out, nil
}

// AddHost register a host to store,calls add_host.sh,
// the host able to connect with store and shared store space.
func (h *hitachiStore) AddHost(name string, wwwn ...string) error {
	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "add_host.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" add host")
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}
	param = append(param, wwwn...)

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" add host")
	}

	output, err := cmd.Output()
	fmt.Printf("exec:%s %s\n%s,error=%v\n", cmd.Path, cmd.Args, output, err)
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	return nil
}

// DelHost deregister host,calls del_host.sh
func (h *hitachiStore) DelHost(name string, wwwn ...string) error {
	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "del_host.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete host")
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}
	// param = append(param, wwwn...)

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete host")
	}

	output, err := cmd.Output()
	fmt.Printf("exec:%s %s\n%s,error=%v\n", cmd.Path, cmd.Args, output, err)
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	return nil
}

// Mapping calls create_lunmap.sh,associate LUN with host.
func (h *hitachiStore) Mapping(host, vg, lun string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	l, err := database.GetLUNByID(lun)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" mapping")
	}

	out, err := database.ListHostLunIDByMapping(host)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" mapping")
	}

	find, val := findIdleNum(h.hs.HluStart, h.hs.HluEnd, out)
	if !find {
		return errors.Errorf("%s:no available host LUN ID", h.Vendor())
	}

	err = database.LunMapping(lun, host, vg, val)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" mapping")
	}

	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "create_lunmap.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" mapping")
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit,
		strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val))
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" mapping")
	}

	output, err := cmd.Output()
	fmt.Printf("exec:%s %s\n%s,error=%v\n", cmd.Path, cmd.Args, output, err)
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	return nil
}

// DelMapping disassociate of the lun from host,calls del_lunmap.sh
func (h *hitachiStore) DelMapping(lun string) error {
	l, err := database.GetLUNByID(lun)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	path, err := utils.GetAbsolutePath(false, scriptPath, HITACHI, "del_lunmap.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit,
		strconv.Itoa(l.StorageLunID))
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	output, err := cmd.Output()
	fmt.Printf("exec:%s %s\n%s,error=%v\n", cmd.Path, cmd.Args, output, err)
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	err = database.DelLunMapping(lun, "", "", 0)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	return nil
}

// AddSpace add a new RG already existed in the store,
func (h *hitachiStore) AddSpace(id int) (Space, error) {
	_, err := database.GetRaidGroup(h.ID(), id)
	if err == nil {
		return Space{}, errors.Errorf("RaidGroup %d is exist in %s", id, h.ID())
	}

	insert := func() error {
		rg := database.RaidGroup{
			ID:          utils.Generate32UUID(),
			StorageID:   h.ID(),
			StorageRGID: id,
			Enabled:     true,
		}

		return rg.Insert()
	}

	// scan RaidGroup info
	h.lock.RLock()
	defer h.lock.RUnlock()

	spaces, err := h.list(id)
	if err != nil {
		return Space{}, errors.Wrap(err, h.Vendor()+" add space")
	}

	for i := range spaces {
		if spaces[i].ID == id {

			if err = insert(); err == nil {

				return spaces[i], nil
			}

			return Space{}, errors.Wrap(err, h.Vendor()+" add space")
		}
	}

	return Space{}, errors.Errorf("%s:Space %d is not exist", h.ID(), id)
}

// EnableSpace signed RG enabled
func (h *hitachiStore) EnableSpace(id int) error {
	h.lock.Lock()
	err := database.UpdateRaidGroupStatus(h.ID(), id, true)
	h.lock.Unlock()

	if err != nil {
		return errors.Wrap(err, h.Vendor()+" enable space")
	}

	return nil
}

// DisableSpace signed RG disabled
func (h *hitachiStore) DisableSpace(id int) error {
	h.lock.Lock()
	err := database.UpdateRaidGroupStatus(h.ID(), id, false)
	h.lock.Unlock()

	if err != nil {
		return errors.Wrap(err, h.Vendor()+" disable space")
	}

	return nil
}

// Info returns store infomation
func (h hitachiStore) Info() (Info, error) {
	list, err := h.Size()
	if err != nil {
		return Info{}, errors.Wrap(err, h.Vendor()+" info")
	}
	info := Info{
		ID:     h.ID(),
		Vendor: h.Vendor(),
		Driver: h.Driver(),
		List:   make(map[int]Space, len(list)),
	}

	for rg, val := range list {
		info.List[rg.StorageRGID] = val
		info.Total += val.Total
		info.Free += val.Free
	}

	return info, nil
}
