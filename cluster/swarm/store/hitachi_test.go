package store

import (
	"testing"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
)

func TestHitachiPing(t *testing.T) {
	hitachiStore := NewHitachiStore("ID0001", "vendor001", "admin001", 1001, 1002, 1003, 1004)
	err := hitachiStore.Ping()
	if err != nil {
		t.Fatal(err)
	}
}

func TestHitachiInsert(t *testing.T) {
	hitachiStore := NewHitachiStore("ID0001", "vendor001", "admin001", 1001, 1002, 1003, 1004)
	err := hitachiStore.Insert()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db, err := database.GetDB(true)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec("DELETE FROM tb_storage_HITACHI WHERE id=?", "ID0001")
		if err != nil {
			t.Fatal(err)
		}
	}()
}

func TestHitachiAlloc(t *testing.T) {
	hitachiStore := NewHitachiStore("ID0001", "vendor001", "admin001", 1001, 1002, 1003, 1004)

	loop := 10
	for i := 0; i < loop; i++ {
		go func(i int) {
			lunID, err := hitachiStore.Alloc("LunName"+string(i), "Unit0001", "unit0001_VG", 1<<30)
			if err != nil {
				t.Fatal(err)
			}
			// clear
			err = hitachiStore.Recycle(lunID, 0)
			if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	time.Sleep(10 * time.Second)
}

func TestHitachiHost(t *testing.T) {
	hitachiStore := NewHitachiStore("ID0001", "vendor001", "admin001", 1001, 1002, 1003, 1004)

	loop := 10
	for i := 0; i < loop; i++ {
		go func(i int) {
			index := string(i)
			err := hitachiStore.AddHost("Name"+index, "wwwnA"+index, "wwwnB"+index)
			if err != nil {
				t.Fatal(err)
			}
			err = hitachiStore.DelHost("Name"+index, "wwwnA"+index, "wwwnB"+index)
			if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	time.Sleep(10 * time.Second)
}

func TestHitachiMapping(t *testing.T) {
	hitachiStore := NewHitachiStore("ID0001", "vendor001", "admin001", 1001, 1002, 1003, 1004)

	loop := 10
	for i := 0; i < loop; i++ {
		go func(i int) {
			index := string(i)
			err := hitachiStore.Mapping("host"+index, "unit"+index, "lun"+index)
			if err != nil {
				t.Fatal(err)
			}
			err = hitachiStore.DelMapping("lun" + index)
			if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	time.Sleep(10 * time.Second)
}

func TestHitachiSpace(t *testing.T) {
	hitachiStore := NewHitachiStore("ID0001", "vendor001", "admin001", 1001, 1002, 1003, 1004)

	loop := 10
	for i := 0; i < loop; i++ {
		go func(i int) {
			space, err := hitachiStore.AddSpace(i)
			if err != nil {
				t.Fatal(err)
			}
			if space <= 0 {
				t.Fatal("space <= 0")
			}
			err = hitachiStore.EnableSpace(i)
			if err != nil {
				t.Fatal(err)
			}
			err = hitachiStore.DisableSpace(i)
			if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	time.Sleep(10 * time.Second)
}
