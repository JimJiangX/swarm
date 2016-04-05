package store

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

const HUAWEI = "HUAWEI"

type huaweiStore struct {
	lock *sync.RWMutex
	hs   database.HuaweiStorage
}

func NewHuaweiStore(id, vendor, addr, user, password string, start, end int) Store {
	return &huaweiStore{
		lock: new(sync.RWMutex),
		hs: database.HuaweiStorage{
			ID:       id,
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
	return "lvm"
}

func (h *huaweiStore) Insert() error {
	h.lock.Lock()

	err := h.hs.Insert()

	h.lock.Unlock()

	return err
}

func (h *huaweiStore) Alloc(_ string, size int) (string, int, error) {
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

	uuid := utils.Generate32UUID()
	lun := database.LUN{
		ID:              uuid,
		Name:            "DBAAS_" + string(uuid[:8]),
		RaidGroupID:     rg.ID,
		StorageSystemID: h.ID(),
		SizeByte:        size,
		StorageLunID:    0,
		CreatedAt:       time.Now(),
	}

	path, err := getAbsolutePath(HUAWEI, "create_lun.sh")

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password,
		strconv.Itoa(rg.StorageRGID), lun.Name, strconv.Itoa(int(size))}

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return "", 0, err
	}

	output, err := cmd.Output()
	if err != nil {
		return "", 0, err
	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	lun.StorageLunID, err = strconv.Atoi(string(output))
	if err != nil {
		return "", 0, err
	}

	err = database.InsertLUN(lun)
	if err != nil {
		return "", 0, err
	}

	return lun.ID, lun.StorageLunID, nil

}

func (h *huaweiStore) Recycle(lun int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	l, err := database.GetLUNByLunID(h.ID(), lun)
	if err != nil {
		return err
	}

	path, err := getAbsolutePath(HUAWEI, "del_lun.sh")
	if err != nil {
		return err
	}

	cmd, err := utils.ExecScript(path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(lun))
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	err = database.DelLUN(l.ID)

	return err
}

func (h huaweiStore) IdleSize() ([]int, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	rg, err := h.idleSize()
	if err != nil {
		return nil, err
	}

	out, i := make([]int, len(rg)), 0
	for _, val := range rg {
		out[i] = val.free
		i++
	}

	return out, nil
}

func (h *huaweiStore) AddHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	path, err := getAbsolutePath(HUAWEI, "add_host.sh")
	if err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, name}
	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	return nil
}

func (h *huaweiStore) DelHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	path, err := getAbsolutePath(HUAWEI, "del_host.sh")
	if err != nil {
		return err
	}

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, name}

	h.lock.Lock()
	defer h.lock.Unlock()

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {

	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	return nil
}

func (h *huaweiStore) Mapping(host, unit, lun string) error {
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

	path, err := getAbsolutePath(HUAWEI, "create_lunmap.sh")
	if err != nil {
		return err
	}

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val)}
	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	return nil
}

func (h *huaweiStore) DelMapping(lun string) error {
	l, err := database.GetLUNByID(lun)
	if err != nil {
		return err
	}

	path, err := getAbsolutePath(HUAWEI, "del_lunmap.sh")
	if err != nil {
		return err
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	param := []string{path, h.hs.IPAddr, h.hs.Username, h.hs.Password, strconv.Itoa(l.StorageLunID)}
	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	err = database.DelLunMapping(lun, "", "", 0)

	return nil
}

func (h *huaweiStore) AddSpace(id int) (int, error) {
	_, err := database.GetRaidGroup(h.ID(), id)
	if err == nil {
		return 0, fmt.Errorf("RaidGroup %d is Exist", id)
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

	spaces, err := h.List()
	if err != nil {
		return 0, err
	}

	for i := range spaces {
		if spaces[i].id == id {

			if err := insert(); err == nil {
				return spaces[i].free, nil
			} else {
				return 0, err
			}
		}
	}

	return 0, fmt.Errorf("Space %d Not Exist", id)
}

func (h *huaweiStore) List(rg ...int) ([]space, error) {
	list := ""
	if len(rg) == 0 {
		return nil, nil

	} else if len(rg) == 1 {
		list = strconv.Itoa(rg[0])
	} else {
		list = intSliceToString(rg, " ")
	}

	path, err := getAbsolutePath(HUAWEI, "listrg.sh")
	if err != nil {
		return nil, err
	}

	cmd, err := utils.ExecScript(path, h.hs.IPAddr, h.hs.Username, h.hs.Password, list)
	if err != nil {
		return nil, err
	}

	output, err := cmd.Output()
	if err != nil {
		fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))
		return nil, err
	}

	spaces := parseSpace(string(output))
	if len(spaces) == 0 {
		return nil, nil
	}

	return spaces, nil
}

func (h *huaweiStore) EnableSpace(id int) error {
	h.lock.Lock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, true)

	h.lock.Unlock()

	return err
}

func (h *huaweiStore) DisableSpace(id int) error {
	h.lock.Lock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, false)

	h.lock.Unlock()

	return err
}

func (h huaweiStore) idleSize() (map[*database.RaidGroup]space, error) {
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
