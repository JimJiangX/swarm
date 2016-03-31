package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

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

func (h *hitachiStore) Insert() error {
	h.lock.Lock()

	err := h.hs.Insert()

	h.lock.Unlock()

	return err
}

func (h *hitachiStore) Alloc(size int64) (int, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	return 0, nil
}

func (h *hitachiStore) Recycle(lun int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h hitachiStore) IdleSize() ([]int64, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	return nil, nil
}

func (h *hitachiStore) AddHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	root, err := os.Getwd()
	if err != nil {
		return err
	}

	path := filepath.Join(root, "HITACHI", "add_host.sh")

	parameter := []string{path, h.hs.AdminUnit, name}
	parameter = append(parameter, wwwn...)

	cmd, err := utils.ExecScript(parameter...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {

	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	return err
}

func (h *hitachiStore) DelHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	root, err := os.Getwd()
	if err != nil {
		return err
	}

	path := filepath.Join(root, "HITACHI", "del_host.sh")

	param := []string{path, h.hs.AdminUnit, name}
	param = append(param, wwwn...)

	cmd, err := utils.ExecScript(param...)
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {

	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

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

	root, err := os.Getwd()
	if err != nil {
		return err
	}

	path := filepath.Join(root, "HITACHI", "create_lunmap.sh")

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit, strconv.Itoa(l.StorageLunID), host, strconv.Itoa(val))
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {

	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	return err
}

func (h *hitachiStore) DelMapping(lun string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	l, err := database.GetLUNByID(lun)
	if err != nil {
		return err
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}

	path := filepath.Join(root, "HITACHI", "del_lunmap.sh")

	cmd, err := utils.ExecScript(path, h.hs.AdminUnit, strconv.Itoa(l.StorageLunID))
	if err != nil {
		return err
	}

	output, err := cmd.Output()
	if err != nil {

	}

	fmt.Println("Exec Script Error:%s,Output:%s", err, string(output))

	err = database.DelLunMapping(lun, "", "", 0)

	return err
}

func (h *hitachiStore) AddSpace(id int) (int64, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	_, err := database.GetRaidGroup(h.ID(), id)
	if err == nil {
		return 0, fmt.Errorf("RaidGroup %d is Exist", id)
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

	// scan RaidGroup info

	return 0, nil
}

func (h *hitachiStore) EnableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, true)

	return err
}

func (h *hitachiStore) DisableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, false)

	return err
}

func findIdleNum(min, max int, filter []int) (bool, int) {
	sort.Sort(sort.IntSlice(filter))

loop:
	for val := min; val <= max; val++ {

		for _, in := range filter {
			if val == in {
				continue loop
			}
		}

		return true, val
	}

	return false, 0
}
