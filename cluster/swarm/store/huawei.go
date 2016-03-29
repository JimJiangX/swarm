package store

import (
	"sync"

	"github.com/docker/swarm/cluster/swarm/database"
)

type huaweiStore struct {
	lock *sync.Mutex
	hs   database.HuaweiStorage
}

func NewHuaweiStore(id, vendor, addr, user, password string, start, end int) Store {
	return &huaweiStore{
		lock: new(sync.Mutex),
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
	return ""
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
	h.lock.Lock()
	defer h.lock.Unlock()

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

	return 0, nil
}

func (h *huaweiStore) EnableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *huaweiStore) DisableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}
