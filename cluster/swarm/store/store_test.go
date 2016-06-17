package store

import (
	"testing"

	"github.com/docker/swarm/cluster/swarm/database"
)

func init() {
	dbSource := "root:111111@tcp(127.0.0.1:3306)/DBaaS?parseTime=true&charset=utf8&loc=Asia%%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'"
	driverName := "mysql"
	database.MustConnect(driverName, dbSource)
}

func TestHITACHIStore(t *testing.T) {
	store, err := RegisterStore("HiTaChI", "", "", "", "AMS2100_83004824", 0, 255, 1000, 1200)
	if err != nil {
		t.Error(HITACHI, err)
	}
	// huawei, err := RegisterStore("hUaWeI", "146.240.104.61", "admin", "Admin@storage", "", 0, 255, 1000, 1200)
	// if err != nil {
	//	t.Error(HUAWEI, err)
	// }

	if store.Vendor() != HITACHI {
		t.Error("Unexpected,want %s got %s", HITACHI, store.Vendor())
	}
	if store.Driver() != SAN_StoreDriver {
		t.Error("Unexpected,want %s got %s", SAN_StoreDriver, store.Driver())
	}

	size, err := store.AddSpace(0)
	if err != nil {
		t.Error(err, size)
	}
	t.Log(0, size)

	spaces, err := store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	err = store.DisableSpace(0)
	if err != nil {
		t.Error(err)
	}
	err = store.EnableSpace(0)
	if err != nil {
		t.Error(err)
	}
	err = store.AddHost("node001")
	if err != nil {
		t.Error(err)
	}

	id, err := store.Alloc("test001", "unit001", "vgName001", 1<<20)
	if err != nil {
		t.Error(err)
	}

	spaces, err = store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	err = store.Mapping("node001", "vgName001", id)
	if err != nil {
		t.Error(err)
	}

	err = store.DelMapping(id)
	if err != nil {
		t.Error(err)
	}

	err = store.Recycle(id, 0)
	if err != nil {
		t.Error(err)
	}

	spaces, err = store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	store.DelHost("node001")
	if err != nil {
		t.Error(err)
	}

}


func TestHUAWEIStore(t *testing.T) {
	 huawei, err := RegisterStore("hUaWeI", "146.240.104.61", "admin", "Admin@storage", "", 0, 255, 1000, 1200)
	 if err != nil {
		t.Error(HUAWEI, err)
	 }

	if store.Vendor() != HUAWEI {
		t.Error("Unexpected,want %s got %s", HUAWEI, store.Vendor())
	}
	if store.Driver() != SAN_StoreDriver {
		t.Error("Unexpected,want %s got %s", SAN_StoreDriver, store.Driver())
	}

	size, err := store.AddSpace(0)
	if err != nil {
		t.Error(err, size)
	}
	t.Log(0, size)

	spaces, err := store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	err = store.DisableSpace(0)
	if err != nil {
		t.Error(err)
	}
	err = store.EnableSpace(0)
	if err != nil {
		t.Error(err)
	}
	err = store.AddHost("node001")
	if err != nil {
		t.Error(err)
	}

	id, err := store.Alloc("test001", "unit001", "vgName001", 1<<20)
	if err != nil {
		t.Error(err)
	}

	spaces, err = store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	err = store.Mapping("node001", "vgName001", id)
	if err != nil {
		t.Error(err)
	}

	err = store.DelMapping(id)
	if err != nil {
		t.Error(err)
	}

	err = store.Recycle(id, 0)
	if err != nil {
		t.Error(err)
	}

	spaces, err = store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	store.DelHost("node001")
	if err != nil {
		t.Error(err)
	}

}
