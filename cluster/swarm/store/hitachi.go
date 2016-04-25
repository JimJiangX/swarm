package store

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

var HITACHI = filepath.Join("script", "HITACHI")

type hitachiStore struct {
	lock *sync.RWMutex
	hs   database.HitachiStorage
}

func NewHitachiStore(id, vendor, admin string, lstart, lend, hstart, hend int) Store {
	return &hitachiStore{
		lock: new(sync.RWMutex),
		hs: database.HitachiStorage{
			ID:        id,
			Vendor:    vendor,
			AdminUnit: admin,
			LunStart:  lstart,
			LunEnd:    lend,
			HluStart:  hstart,
			HluEnd:    hend,
		},
	}
}

func (h hitachiStore) ID() string {
	return h.hs.ID
}

func (h hitachiStore) Vendor() string {
	return h.hs.Vendor
}

func (h hitachiStore) Driver() string {
	return "lvm"
}

func (h hitachiStore) Ping() error {
	path, err := utils.GetAbsolutePath(false, HITACHI, "connect_test.sh")
	if err != nil {
		return err
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	return nil
}

func (h *hitachiStore) Insert() error {

	return h.hs.Insert()
}

func (h *hitachiStore) Alloc(name, vendor string, size int) (string, int, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	out, err := h.idleSize()
	if err != nil {
		return "", 0, err
	}

	rg := maxIdleSizeRG(out)
	if out[rg].free < size {
		return "", 0, fmt.Errorf("Not Enough Space For Alloction,Max:%d < Need:%d", out[rg], size)
	}

	used, err := database.SelectLunIDBySystemID(h.ID())
	if err != nil {
		return "", 0, err
	}

	ok, id := findIdleNum(h.hs.LunStart, h.hs.LunEnd, used)
	if !ok {
		return "", 0, fmt.Errorf("No available LUN ID")
	}

	lun := database.LUN{
		ID:              utils.Generate64UUID(),
		Name:            name,
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        size,
		StorageLunID:    id,
		CreatedAt:       time.Now(),
	}

	path, err := utils.GetAbsolutePath(false, HITACHI, "create_lun.sh")
	if err != nil {
		return "", 0, err
	}
	param := []string{path, h.hs.AdminUnit,
		strconv.Itoa(rg.StorageRGID), strconv.Itoa(id), strconv.Itoa(int(size))}

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return "", 0, err
	}

	output, err := cmd.Output()
	if err != nil {
		return "", 0, fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	err = database.InsertLUN(lun)
	if err != nil {
		return "", 0, err
	}

	return lun.ID, lun.StorageLunID, nil
}

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
		return err
	}

	path, err := utils.GetAbsolutePath(false, HITACHI, "del_lun.sh")
	if err != nil {
		return err
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit, strconv.Itoa(l.StorageLunID))
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	err = database.DelLUN(l.ID)

	return err
}

func (h hitachiStore) idleSize() (map[*database.RaidGroup]space, error) {
	out, err := database.SelectRaidGroupByStorageID(h.ID(), true)
	if err != nil {
		return nil, err
	}

	rg := make([]int, len(out))

	for i, val := range out {
		rg[i] = val.StorageRGID
	}

	spaces, err := h.List(rg...)
	if err != nil {
		return nil, err
	}

	var info map[*database.RaidGroup]space

	if len(spaces) > 0 {
		info = make(map[*database.RaidGroup]space)

		for i := range out {
		loop:
			for s := range spaces {
				if out[i].StorageRGID == spaces[s].id {
					info[out[i]] = spaces[s]
					break loop
				}
			}
		}
	}

	return info, nil
}

func (h *hitachiStore) List(rg ...int) ([]space, error) {
	list := ""
	if len(rg) == 0 {
		return nil, nil

	} else if len(rg) == 1 {
		list = strconv.Itoa(rg[0])
	} else {
		list = intSliceToString(rg, " ")
	}

	path, err := utils.GetAbsolutePath(false, HITACHI, "listrg.sh")
	if err != nil {
		return nil, err
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit, list)
	if err != nil {
		return nil, err
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	spaces := parseSpace(string(output))
	if len(spaces) == 0 {
		return nil, nil
	}

	return spaces, nil
}

func (h hitachiStore) IdleSize() (map[string]int, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	rg, err := h.idleSize()
	if err != nil {
		return nil, err
	}

	out := make(map[string]int, len(rg))
	for key, val := range rg {
		out[key.ID] = val.free
	}

	return out, nil
}

func (h *hitachiStore) AddHost(name string, wwwn ...string) error {
	path, err := utils.GetAbsolutePath(false, HITACHI, "add_host.sh")
	if err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}
	param = append(param, wwwn...)

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	return err
}

func (h *hitachiStore) DelHost(name string, wwwn ...string) error {
	path, err := utils.GetAbsolutePath(false, HITACHI, "del_host.sh")
	if err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.AdminUnit, name}
	param = append(param, wwwn...)

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	return err
}

func (h *hitachiStore) Mapping(host, unit, lun string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	l, err := database.GetLUNByID(lun)
	if err != nil {
		return err
	}

	out, err := database.SelectHostLunIDByMapping(host)
	if err != nil {
		return err
	}

	find, val := findIdleNum(h.hs.HluStart, h.hs.HluEnd, out)
	if !find {
		return fmt.Errorf("No available Host LUN ID")
	}

	err = database.LunMapping(lun, host, unit, val)
	if err != nil {
		return err
	}

	path, err := utils.GetAbsolutePath(false, HITACHI, "create_lunmap.sh")
	if err != nil {
		return err
	}

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit,
		strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val))
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	return err
}

func (h *hitachiStore) DelMapping(lun string) error {
	l, err := database.GetLUNByID(lun)
	if err != nil {
		return err
	}

	path, err := utils.GetAbsolutePath(false, HITACHI, "del_lunmap.sh")
	if err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit,
		strconv.Itoa(l.StorageLunID))
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Exec Script Error:%s,Output:%s", err, string(output))
	}

	err = database.DelLunMapping(lun, "", "", 0)

	return err
}

func (h *hitachiStore) AddSpace(id int) (int, error) {

	_, err := database.GetRaidGroup(h.ID(), id)
	if err == nil {
		return 0, fmt.Errorf("RaidGroup %d is Exist", id)
	}

	// scan RaidGroup info
	h.lock.RLock()
	defer h.lock.RUnlock()

	spaces, err := h.List(id)
	if err != nil {
		return 0, err
	}

	if len(spaces) != 1 {

		return 0, fmt.Errorf("Error happens when scan space %d", id)
	}

	rg := database.RaidGroup{
		ID:          utils.Generate32UUID(),
		StorageID:   h.ID(),
		StorageRGID: id,
		Enabled:     true,
	}

	err = rg.Insert()
	if err != nil {
		return 0, err
	}

	return spaces[0].free, nil
}

func (h *hitachiStore) EnableSpace(id int) error {
	h.lock.Lock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, true)

	h.lock.Unlock()

	return err
}

func (h *hitachiStore) DisableSpace(id int) error {
	h.lock.Lock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, false)

	h.lock.Unlock()

	return err
}
