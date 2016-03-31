package store

import (
	"fmt"
	"os"
	"path/filepath"
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

func (h *hitachiStore) Mapping(host, unit string, lun int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *hitachiStore) DelMapping(host string, lun int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *hitachiStore) AddSpace(id int) (int64, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	rg := database.RaidGroup{
		ID:          utils.Generate32UUID(),
		StorageID:   h.ID(),
		StorageRGID: id,
		Enabled:     true,
	}

	err := rg.Insert()
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
	if err != nil {
		return err
	}

	return nil
}

func (h *hitachiStore) DisableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, false)
	if err != nil {
		return err
	}

	return nil
}
