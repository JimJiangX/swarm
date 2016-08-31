package storage

import (
	"os"
	"testing"

	"github.com/docker/swarm/cluster/swarm/database"
)

func init() {
	dbSource := "root:111111@tcp(127.0.0.1:3306)/DBaaS?parseTime=true&charset=utf8&loc=Asia%%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'"
	driverName := "mysql"
	database.Connect(driverName, dbSource)
	// set Current workdir to be script dir
	os.Chdir("/")
}

func TestRegisterStore(t *testing.T) {
	wrong, err := RegisterStore("HiaChI", "", "", "", "AMS2100_83004824", 0, 255, 1000, 1200)
	if err == nil {
		t.Error(wrong.Vendor())
	}
	t.Log("Expected,", err)
	hitachi, err := RegisterStore("HiTaChI", "", "", "", "AMS2100_83004824", 0, 255, 1000, 1200)
	if err != nil {
		t.Error(HITACHI, err)
	} else {
		t.Log(hitachi.ID(), HITACHI, "registered")
	}

	huawei, err := RegisterStore("hUaWeI", "146.240.104.61", "admin", "Admin@storage", "", 0, 255, 1000, 1200)
	if err != nil {
		t.Error(HUAWEI, err)
	} else {
		t.Log(huawei.ID(), HUAWEI, "registered")
	}

	if hitachi != nil {
		database.DeleteStorageByID(hitachi.ID())
	}
	if huawei != nil {
		database.DeleteStorageByID(huawei.ID())
	}
}

func TestHITACHIStore(t *testing.T) {
	store, err := RegisterStore("HiTaChI", "", "", "", "AMS2100_83004824", 0, 255, 1000, 1200)
	if err != nil {
		t.Error(HITACHI, err)
	}

	if store.Vendor() != HITACHI {
		t.Errorf("Unexpected,want %s got %s", HITACHI, store.Vendor())
	}
	if store.Driver() != SANStoreDriver {
		t.Errorf("Unexpected,want %s got %s", SANStoreDriver, store.Driver())
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

	lun, lv, err := store.Alloc("test001", "unit001", "vgName001", 1<<20)
	if err != nil {
		t.Error(err)
	}
	t.Log(lun, lv)

	spaces, err = store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	err = store.Mapping("node001", "vgName001", lun.ID)
	if err != nil {
		t.Error(err)
	}

	err = store.DelMapping(lun.ID)
	if err != nil {
		t.Error(err)
	}

	err = store.Recycle(lun.ID, 0)
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

	database.DeleteStorageByID(store.ID())
}

func TestHUAWEIStore(t *testing.T) {
	store, err := RegisterStore("hUaWeI", "146.240.104.61", "admin", "Admin@storage", "", 0, 255, 1000, 1200)
	if err != nil {
		t.Error(HUAWEI, err)
	}

	if store.Vendor() != HUAWEI {
		t.Errorf("Unexpected,want %s got %s", HUAWEI, store.Vendor())
	}
	if store.Driver() != SANStoreDriver {
		t.Errorf("Unexpected,want %s got %s", SANStoreDriver, store.Driver())
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

	lun, lv, err := store.Alloc("test001", "unit001", "vgName001", 1<<20)
	if err != nil {
		t.Error(err)
	}
	t.Log(lun, lv)

	spaces, err = store.IdleSize()
	if err != nil {
		t.Error(err)
	}

	for key, val := range spaces {
		t.Log(key, val)
	}

	err = store.Mapping("node001", "vgName001", lun.ID)
	if err != nil {
		t.Error(err)
	}

	err = store.DelMapping(lun.ID)
	if err != nil {
		t.Error(err)
	}

	err = store.Recycle(lun.ID, 0)
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

	database.DeleteStorageByID(store.ID())
}

func TestGetStoreByID(t *testing.T) {
	lock.RLock()
	n := len(stores)
	lock.RUnlock()

	if n == 0 {
		store, err := RegisterStore("HiTaChI", "", "", "", "AMS2100_83004824", 0, 255, 1000, 1200)
		if err != nil {
			t.Error(HITACHI, err)
		}
		t.Log(store.ID(), HITACHI, "registered")
		store, err = RegisterStore("hUaWeI", "146.240.104.61", "admin", "Admin@storage", "", 0, 255, 1000, 1200)
		if err != nil {
			t.Error(HUAWEI, err)
		}
		t.Log(store.ID(), HUAWEI, "registered")
	}
	list := make([]string, 0, n)

	lock.Lock()
	for id := range stores {
		list = append(list, id)
		delete(stores, id)
	}
	lock.Unlock()

	for i := range list {
		store, err := GetStore(list[i])
		if err != nil || store == nil {
			t.Error("failed to get store", list[i])
		}
	}
}
