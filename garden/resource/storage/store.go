package storage

import (
	"strings"
	"sync"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

const (
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
	Ping() error
	IdleSize() (map[string]int64, error)

	Insert() error

	AddHost(name string, wwwn ...string) error
	DelHost(name string, wwwn ...string) error

	Alloc(name, unitID, vgName string, size int64) (database.LUN, database.Volume, error) // create LUN
	Extend(lv database.Volume, size int64) (database.LUN, database.Volume, error)
	Recycle(id string, lun int) error // delete LUN

	Mapping(host, vgName, lun string) error
	DelMapping(lun string) error

	AddSpace(id string) (Space, error)
	EnableSpace(id string) error
	DisableSpace(id string) error
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

// RegisterStore register a new remote storage system
func (s *stores) Add(vendor, addr, user, password, admin string,
	lstart, lend, hstart, hend int) (store Store, err error) {

	switch strings.ToUpper(vendor) {
	case HUAWEI:
		hs := database.HuaweiStorage{
			ID:       utils.Generate32UUID(),
			Vendor:   HUAWEI,
			IPAddr:   addr,
			Username: user,
			Password: password,
			HluStart: hstart,
			HluEnd:   hend,
		}
		store = newHuaweiStore(s.orm, s.script, hs)
	case HITACHI:
		hs := database.HitachiStorage{
			ID:        utils.Generate32UUID(),
			Vendor:    HITACHI,
			AdminUnit: admin,
			LunStart:  lstart,
			LunEnd:    lend,
			HluStart:  hstart,
			HluEnd:    hend,
		}
		store = newHitachiStore(s.orm, s.script, hs)
	default:
		return nil, errors.Errorf("unsupported Vendor '%s' yet", vendor)
	}

	if err := store.Ping(); err != nil {
		return store, err
	}

	if err := store.Insert(); err != nil {
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

	hitachi, huawei, err := s.orm.GetStorageByID(ID)
	if err != nil {
		return nil, errors.Wrap(err, "get store by ID")
	}

	if hitachi != nil {
		store = newHitachiStore(s.orm, s.script, *hitachi)
	} else if huawei != nil {
		store = newHuaweiStore(s.orm, s.script, *huawei)
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

	return s.orm.DelRaidGroup(store.ID(), space)
}
