package store

import (
	"strings"
	"sync"

	"github.com/docker/swarm/cluster/swarm/database"
)

const (
	scriptPath = "script"
	HITACHI    = "HITACHI"
	HUAWEI     = "HUAWEI"

	LocalStorePrefix = "local"
	SANStore         = "san"

	SAN_StoreDriver  = "lvm"
	LocalStoreDriver = "lvm"

	DefaultFilesystemType = "xfs"
)

var stores map[string]Store = make(map[string]Store)

type Store interface {
	ID() string
	Vendor() string
	Driver() string
	Ping() error
	IdleSize() (map[string]int, error)

	Insert() error

	AddHost(name string, wwwn ...string) error
	DelHost(name string, wwwn ...string) error

	Alloc(name, unitID, vgName string, size int) (string, error) // create LUN
	Recycle(id string, lun int) error                            // delete LUN

	Mapping(host, vgName, lun string) error
	DelMapping(lun string) error

	AddSpace(id int) (int, error)
	EnableSpace(id int) error
	DisableSpace(id int) error
}

func RegisterStore(vendor, addr, user, password, admin string,
	lstart, lend, hstart, hend int) (store Store, err error) {

	if v := strings.ToUpper(vendor); HUAWEI == v {
		store = NewHuaweiStore(HUAWEI, addr, user, password, hstart, hend)
	} else if HITACHI == v {
		store = NewHitachiStore(HITACHI, admin, lstart, lend, hstart, hend)
	}

	if err := store.Ping(); err != nil {
		return nil, err
	}

	if err := store.Insert(); err != nil {
		return nil, err
	}

	stores[store.ID()] = store

	return nil, nil
}

func GetStoreByID(ID string) (Store, error) {
	store, ok := stores[ID]
	if ok && store != nil {
		return store, nil
	}

	hitachi, huawei, err := database.GetStorageByID(ID)
	if err != nil {
		return nil, err
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
		stores[store.ID()] = store
	}

	return store, nil
}
