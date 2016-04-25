package store

import (
	"strings"
	"sync"

	"github.com/docker/swarm/cluster/swarm/database"
)

const scriptPath = "script"

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

	Alloc(name, vendor string, size int) (string, int, error) // create LUN
	Recycle(id string, lun int) error                         // delete LUN

	Mapping(host, unit, lun string) error
	DelMapping(lun string) error

	AddSpace(id int) (int, error)
	EnableSpace(id int) error
	DisableSpace(id int) error
}

func RegisterStore(id, vendor, addr, user, password, admin string,
	lstart, lend, hstart, hend int) (store Store, err error) {

	if v := strings.ToUpper(vendor); HUAWEI == v {
		store = NewHuaweiStore(id, HUAWEI, addr, user, password, hstart, hend)
	} else if HITACHI == v {
		store = NewHitachiStore(id, HITACHI, admin, lstart, lend, hstart, hend)
	} else if vendor == LocalDiskStore {
		return nil, nil
	}

	if err := store.Ping(); err != nil {
		return nil, err
	}

	if err := store.Insert(); err != nil {
		return nil, err
	}

	stores[id] = store

	return nil, nil
}

func GetStoreByID(ID string) (Store, error) {
	hitachi, huawei, err := database.GetStorageByID(ID)
	if err != nil {
		return nil, err
	}

	if hitachi != nil {
		return &hitachiStore{
			lock: new(sync.RWMutex),
			hs:   *hitachi,
		}, nil
	}

	if huawei != nil {
		return &huaweiStore{
			lock: new(sync.RWMutex),
			hs:   *huawei,
		}, nil
	}

	return nil, nil
}
