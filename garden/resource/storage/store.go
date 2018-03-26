package storage

import (
	"strings"
	"sync"
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

const (
	defaultTimeout = 5 * time.Minute

	defaultScriptPath = "./script"
	// HITACHI store vendor name
	HITACHI = "HITACHI"
	// HUAWEI store vendor name
	HUAWEI = "HUAWEI"
	// LocalStorePrefix prefix of local store
	LocalStorePrefix = "local"
	// SANStore type
	SANStore = "SAN"

	// SANStoreDriver SAN store driver
	SANStoreDriver = "lvm"
	// LocalStoreDriver local store Driver
	LocalStoreDriver = "lvm"

	// DefaultFilesystemType default filesystem type
	DefaultFilesystemType = "xfs"

	maxHostLen = 30
)

// Info describle remote storage system infomation
type Info struct {
	ID     string
	Vendor string
	Driver string
	Fstype string
	Total  int64
	Free   int64
	List   map[string]Space
}

// Store is remote storage system
type Store interface {
	Info() (Info, error)
	ID() string
	Vendor() string
	Driver() string
	ping() error
	//	IdleSize() (map[string]int64, error)
	ListLUN(nameOrVG string) ([]database.LUN, error)

	insert() error

	AddHost(name string, wwwn ...string) error
	DelHost(name string, wwwn ...string) error

	Alloc(name, unit, vgName, host string, size int64) (database.LUN, database.Volume, error) // create LUN
	Extend(lv database.Volume, size int64) (database.LUN, database.Volume, error)
	RecycleLUN(id string, lun int) error // delete LUN

	Mapping(host, vgName, lun, unit string) (database.LUN, error)
	DelMapping(lun database.LUN) error

	AddSpace(id string) (Space, error)
	EnableSpace(id string) error
	DisableSpace(id string) error
	removeSpace(id string) error
}

var defaultStores *stores

type stores struct {
	lock *sync.RWMutex

	script string

	orm database.StorageOrmer

	stores map[string]Store
}

// SetDefaultStores set defaultStores,function should calls before call DefaultStores()
func SetDefaultStores(script string, orm database.StorageOrmer) {
	if script == "" {
		script = defaultScriptPath
	}

	defaultStores = &stores{
		script: script,
		orm:    orm,
		lock:   new(sync.RWMutex),
		stores: make(map[string]Store),
	}
}

// DefaultStores returns defaultStores
func DefaultStores() *stores {
	return defaultStores
}

func newStore(orm database.StorageOrmer, script string, san database.SANStorage) (store Store, err error) {
	switch strings.ToUpper(san.Vendor) {
	case HUAWEI:
		san.Vendor = HUAWEI
		store = newHuaweiStore(orm, script, san)
	case HITACHI:
		san.Vendor = HITACHI
		store = newHitachiStore(orm, script, san)
	default:
		return nil, errors.Errorf("unsupported Vendor '%s' yet", san.Vendor)
	}

	return store, nil
}

// RegisterStore register a new remote storage system
func (s *stores) Add(vendor, version, addr, user, password, admin string,
	lstart, lend, hstart, hend int) (Store, error) {

	san := database.SANStorage{
		ID:        utils.Generate32UUID(),
		Vendor:    vendor,
		Version:   version,
		AdminUnit: admin,
		LunStart:  lstart,
		LunEnd:    lend,
		HluStart:  hstart,
		HluEnd:    hend,
	}

	store, err := newStore(s.orm, s.script, san)
	if err != nil {
		return nil, err
	}

	if err := store.ping(); err != nil {
		return store, err
	}

	if err := store.insert(); err != nil {
		return store, err
	}

	s.lock.Lock()
	s.stores[store.ID()] = store
	s.lock.Unlock()

	return store, nil
}

// GetStore returns store find by ID
func (s *stores) Get(ID string) (Store, error) {
	s.lock.RLock()
	store, ok := s.stores[ID]
	s.lock.RUnlock()

	if ok && store != nil {
		return store, nil
	}

	san, err := s.orm.GetStorageByID(ID)
	if err != nil {
		return nil, err
	}

	store, err = newStore(s.orm, s.script, san)
	if err != nil {
		return nil, err
	}

	if store != nil {
		s.lock.Lock()
		s.stores[store.ID()] = store
		s.lock.Unlock()
	}

	return store, nil
}

func (s *stores) List() ([]Store, error) {
	ids, err := s.orm.ListStorageID()
	if err != nil {
		return nil, err
	}

	list := make([]Store, 0, len(ids))

	for i := range ids {
		store, err := s.Get(ids[i])
		if err != nil {
			return nil, err
		}

		list = append(list, store)
	}

	return list, nil
}

// RemoveStore removes the assigned store,
// if store is using,cannot be remove
func (s *stores) Remove(ID string) error {
	err := s.orm.DelRGCondition(ID)
	if err != nil {
		return err
	}

	return s.remove(ID)
}

func (s *stores) remove(ID string) error {
	s.lock.Lock()
	delete(s.stores, ID)
	s.lock.Unlock()

	return s.orm.DelStorageByID(ID)
}

// RemoveStoreSpace removes a Space of store,
// if Space is using,cannot be removed
func (s *stores) RemoveStoreSpace(ID, space string) error {
	store, err := s.Get(ID)
	if err != nil {
		return err
	}

	rg, err := s.orm.GetRaidGroup(store.ID(), space)
	if err != nil {
		return err
	}

	count, err := s.orm.CountLunByRaidGroupID(rg.ID)
	if err != nil {
		return err
	}

	if count > 0 {
		return errors.Errorf("Store %s RaidGroup %s is using,cannot be removed", store.ID(), space)
	}

	return store.removeSpace(space)
}
