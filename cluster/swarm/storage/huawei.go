package storage

import (
	"strconv"
	"sync"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
)

type huaweiStore struct {
	lock *sync.RWMutex
	hs   database.HuaweiStorage
}

// NewHuaweiStore returns a new huawei store
func NewHuaweiStore(vendor, addr, user, password string, start, end int) Store {
	return &huaweiStore{
		lock: new(sync.RWMutex),
		hs: database.HuaweiStorage{
			ID:       utils.Generate64UUID(),
			Vendor:   vendor,
			IPAddr:   addr,
			Username: user,
			Password: password,
			HluStart: start,
			HluEnd:   end,
		},
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

func (h *huaweiStore) Ping() error {
	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "connect_test.sh")
	if err != nil {
		return errors.Wrap(err, "ping store:"+h.Vendor())
	}

	cmd, err := utils.ExecScript(path, h.hs.IPAddr, h.hs.Username, h.hs.Password)
	if err != nil {
		return errors.Wrap(err, "ping store:"+h.Vendor())
	}

	output, err := cmd.Output()
	if err != nil {
		return errors.Wrapf(err, "Exec:%s,Output:%s", cmd.Args, output)
	}

	return nil
}

func (h *huaweiStore) Insert() error {
	h.lock.Lock()
	err := h.hs.Insert()
	h.lock.Unlock()

	if err != nil {
		return errors.Wrap(err, "insert huawei store")
	}

	return nil
}

func (h *huaweiStore) Alloc(name, unit, vg string, size int) (database.LUN, database.LocalVolume, error) {
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

	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "create_lun.sh")
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" store alloc")
	}
	// size:byte-->MB
	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password,
		strconv.Itoa(rg.StorageRGID), name, strconv.Itoa(size>>20 + 100)}

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" alloc LUN")
	}

	output, err := cmd.Output()
	if err != nil {
		return lun, lv, errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	storageLunID, err := strconv.Atoi(string(output))
	if err != nil {
		return lun, lv, errors.Wrap(err, h.Vendor()+" alloc LUN")
	}

	lun = database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            name,
		VGName:          vg,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        size,
		StorageLunID:    storageLunID,
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

func (h *huaweiStore) Recycle(id string, lun int) error {
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

	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "del_lun.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" recycle")
	}

	cmd, err := utils.ExecScript(path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(l.StorageLunID))
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" recycle")
	}

	output, err := cmd.Output()
	if err != nil {
		return errors.Wrapf(err, "Exec:%s,Output:%s", cmd.Args, output)
	}

	err = database.DelLUN(l.ID)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" recycle")
	}

	return nil
}

func (h huaweiStore) IdleSize() (map[string]int, error) {
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

func (h *huaweiStore) AddHost(name string, wwwn ...string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "add_host.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" add host")
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, name}
	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" add host")
	}

	output, err := cmd.Output()
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	return nil
}

func (h *huaweiStore) DelHost(name string, wwwn ...string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "del_host.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete host")
	}

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, name}

	h.lock.Lock()
	defer h.lock.Unlock()

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete host")
	}

	output, err := cmd.Output()
	if err != nil {
		return errors.Errorf("Exec %s:%s,Output:%s", cmd.Args, err, output)
	}

	return nil
}

func (h *huaweiStore) Mapping(host, vg, lun string) error {
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
		return errors.Errorf("%s:no available Host LUN ID", h.Vendor())
	}

	err = database.LunMapping(lun, host, vg, val)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" lun mapping")
	}

	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "create_lunmap.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" lun mapping")
	}

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val)}
	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" lun mapping")
	}

	output, err := cmd.Output()
	if err != nil {
		return errors.Wrapf(err, "Exec:%s,Output:%s", cmd.Args, output)
	}

	return nil
}

func (h *huaweiStore) DelMapping(lun string) error {
	l, err := database.GetLUNByID(lun)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "del_lunmap.sh")
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(l.StorageLunID)}
	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	output, err := cmd.Output()
	if err != nil {
		return errors.Wrapf(err, "Exec:%s,Output:%s", cmd.Args, output)
	}

	err = database.DelLunMapping(lun, "", "", 0)
	if err != nil {
		return errors.Wrap(err, h.Vendor()+" delete mapping")
	}

	return nil
}

func (h *huaweiStore) AddSpace(id int) (Space, error) {
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

func (h *huaweiStore) list(rg ...int) ([]Space, error) {
	list := ""
	if len(rg) == 0 {
		return nil, nil

	} else if len(rg) == 1 {
		list = strconv.Itoa(rg[0])
	} else {
		list = intSliceToString(rg, " ")
	}

	path, err := utils.GetAbsolutePath(false, scriptPath, HUAWEI, "listrg.sh")
	if err != nil {
		return nil, errors.Wrap(err, h.Vendor()+" list")
	}

	cmd, err := utils.ExecScript(path, h.hs.IPAddr, h.hs.Username, h.hs.Password, list)
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

func (h *huaweiStore) EnableSpace(id int) error {
	h.lock.Lock()
	err := database.UpdateRaidGroupStatus(h.ID(), id, true)
	h.lock.Unlock()

	if err != nil {
		return errors.Wrap(err, h.Vendor()+" enable space")
	}

	return nil
}

func (h *huaweiStore) DisableSpace(id int) error {
	h.lock.Lock()
	err := database.UpdateRaidGroupStatus(h.ID(), id, false)
	h.lock.Unlock()

	if err != nil {
		return errors.Wrap(err, h.Vendor()+" disable space")
	}

	return nil
}

func (h huaweiStore) Size() (map[database.RaidGroup]Space, error) {
	out, err := database.ListRGByStorageID(h.ID())
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

func (h huaweiStore) Info() (Info, error) {
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
