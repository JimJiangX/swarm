package store

import (
	"sync"

	"github.com/docker/swarm/cluster/swarm/database"
)

type hitachiStore struct {
	lock *sync.Mutex
	hs   database.HitachiStorage
}

func NewHitachiStore(id, vendor, admin string, lstart, lend, hstart, hend int) Store {
	return &hitachiStore{
		lock: new(sync.Mutex),
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
	return ""
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
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil, nil
}

func (h *hitachiStore) AddHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *hitachiStore) DelHost(name string, wwwn []string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
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

	return 0, nil
}

func (h *hitachiStore) EnableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}

func (h *hitachiStore) DisableSpace(id int) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	return nil
}
