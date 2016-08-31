package storage

import (
	"strings"
	"sync"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/pkg/errors"
)

const (
	scriptPath = "script"
	// HITACHI store vendor name
	HITACHI = "HITACHI"
	// HUAWEI store vendor name
	HUAWEI = "HUAWEI"
	// LocalStorePrefix prefix of local store
	LocalStorePrefix = "local"
	// SANStore type
	SANStore = "san"

	// SANStoreDriver SAN store driver
	SANStoreDriver = "lvm"
	// LocalStoreDriver local store Driver
	LocalStoreDriver = "lvm"

	// DefaultFilesystemType default filesystem type
	DefaultFilesystemType = "xfs"
)

// stores holds all registered Stores
var (
	stores = make(map[string]Store, 10)
	lock   sync.RWMutex
)

// Info describle remote storage system infomation
type Info struct {
	ID     string
	Vendor string
	Driver string
	Total  int
	Free   int
	List   map[int]Space
}

// Store is remote storage system
type Store interface {
	Info() (Info, error)
	ID() string
	Vendor() string
	Driver() string
	Ping() error
	IdleSize() (map[string]int, error)

	Insert() error

	AddHost(name string, wwwn ...string) error
	DelHost(name string, wwwn ...string) error

	Alloc(name, unitID, vgName string, size int) (database.LUN, database.LocalVolume, error) // create LUN
	Recycle(id string, lun int) error                                                        // delete LUN

	Mapping(host, vgName, lun string) error
	DelMapping(lun string) error

	AddSpace(id int) (Space, error)
	EnableSpace(id int) error
	DisableSpace(id int) error
}

// RegisterStore register a new remote storage system
func RegisterStore(vendor, addr, user, password, admin string,
	lstart, lend, hstart, hend int) (store Store, err error) {

	switch strings.ToUpper(vendor) {
	case HUAWEI:
		store = NewHuaweiStore(HUAWEI, addr, user, password, hstart, hend)
	case HITACHI:
		store = NewHitachiStore(HITACHI, admin, lstart, lend, hstart, hend)
	default:
		return nil, errors.Errorf("unsupported Vendor '%s' yet", vendor)
	}

	if err := store.Ping(); err != nil {
		return store, err
	}

	if err := store.Insert(); err != nil {
		return store, err
	}

	lock.Lock()
	stores[store.ID()] = store
	lock.Unlock()

	return store, nil
}

// GetStore returns store find by ID
func GetStore(ID string) (Store, error) {
	lock.RLock()
	store, ok := stores[ID]
	lock.RUnlock()

	if ok && store != nil {
		return store, nil
	}

	hitachi, huawei, err := database.GetStorageByID(ID)
	if err != nil {
		return nil, errors.Wrap(err, "get store by ID")
	}

	if hitachi != nil {
		store = &hitachiStore{
			lock: new(sync.RWMutex),
			hs:   *hitachi,
		}
	} else if huawei != nil {
		store = &huaweiStore{
			lock: new(sync.RWMutex),
			hs:   *huawei,
		}
	}

	if store != nil {
		lock.Lock()
		stores[store.ID()] = store
		lock.Unlock()
	}

	return store, nil
}

// RemoveStore removes the assigned store,
// if store is using,cannot be remove
func RemoveStore(ID string) error {
	count, err := database.CountClusterByStorage(ID)
	if err != nil {
		return err
	}

	if count > 0 {
		return errors.Errorf("Store %s is using,cannot be removed", ID)
	}

	return removeStore(ID)
}

func removeStore(ID string) error {
	lock.Lock()
	delete(stores, ID)
	lock.Unlock()

	return database.DeleteStorageByID(ID)
}

// RemoveStoreSpace removes a Space of store,
// if Space is using,cannot be removed
func RemoveStoreSpace(ID string, space int) error {
	store, err := GetStore(ID)
	if err != nil {
		return err
	}

	rg, err := database.GetRaidGroup(store.ID(), space)
	if err != nil {
		return err
	}

	count, err := database.CountLUNByRaidGroupID(rg.ID)
	if err != nil {
		return err
	}

	if count > 0 {
		return errors.Errorf("Store %s RaidGroup %d is using,cannot be removed", store.ID(), space)
	}

	return database.DeleteRaidGroup(store.ID(), space)
}
