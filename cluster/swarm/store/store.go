package store

var stores map[string]Store = make(map[string]Store)

type Store interface {
	ID() string
	Vendor() string
	Driver() string
	IdleSize() ([]int64, error)

	Insert() error

	AddHost(name string, wwwn []string) error
	DelHost(name string, wwwn []string) error

	Alloc(size int64) (int, error) // create LUN
	Recycle(lun int) error         // delete LUN

	Mapping(host, unit, lun string) error
	DelMapping(lun string) error

	AddSpace(id int) (int64, error)
	EnableSpace(id int) error
	DisableSpace(id int) error
}

func RegisterStore(id, vendor, addr, user, password, admin string,
	lstart, lend, hstart, hend int) (store Store, err error) {

	if vendor == "huawei" {
		store = NewHuaweiStore(id, vendor, addr, user, password, hstart, hend)
		err = store.Insert()

	} else if vendor == "hitachi" {
		store = NewHitachiStore(id, vendor, admin, lstart, lend, hstart, hend)
		err = store.Insert()
	}

	if err != nil {
		return nil, err
	}

	stores[id] = store

	return nil, nil
}
