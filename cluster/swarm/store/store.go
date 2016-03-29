package store

var stores map[string]Store = make(map[string]Store)

type Store interface {
	ID() string
	Vendor() string
	Driver() string
	IdleSize() ([]int64, error)

	AddHost(name string, wwwn []string) error
	DelHost(name string, wwwn []string) error

	Alloc(size int64) (int, error) // create LUN
	Recycle(lun int) error         // delete LUN

	Mapping(host, unit string, lun int) error
	DelMapping(host string, lun int) error

	AddSpace(id int) (int64, error)
	EnableSpace(id int) error
	DisableSpace(id int) error
}

func RegisterStore(id, vendor, addr, user, password, admin string,
	lstart, lend, hstart, hend int) (Store, error) {
	var store Store = nil

	if vendor == "huawei" {

		store = NewHuaweiStore(id, vendor, addr, user, password, lstart, lend)

	} else if vendor == "hitachi" {

		store = NewHitachiStore(id, vendor, admin, lstart, lend, hstart, hend)
	}

	stores[id] = store

	return nil, nil
}
