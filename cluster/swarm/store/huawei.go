package store

import (
	"strconv"
	"sync"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

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

func (h *huaweiStore) Alloc(size int64) (int, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	return 0, nil
}

func (h *huaweiStore) Recycle(lun int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h huaweiStore) IdleSize() ([]int64, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	spaces, err := database.SelectRaidGroupByStorageID(h.hs.ID, true)
	if err != nil {
		return nil, err
	}

	rg := ""

	for _, val := range spaces {
		rg += strconv.Itoa(val.StorageRGID) + " "
	}

	return nil, nil
}

func (h *huaweiStore) AddHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *huaweiStore) DelHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *huaweiStore) Mapping(host, unit string, lun int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *huaweiStore) DelMapping(host string, lun int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *huaweiStore) AddSpace(id int) (int64, error) {
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

func (h *huaweiStore) EnableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, true)
	if err != nil {
		return err
	}

	return nil
}

func (h *huaweiStore) DisableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	err := database.UpdateRaidGroupStatus(h.ID(), id, false)
	if err != nil {
		return err
	}

	return nil
}
