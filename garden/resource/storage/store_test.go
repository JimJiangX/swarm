package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/swarm/garden/database"
	_ "github.com/go-sql-driver/mysql"
)

var (
	db database.Ormer

	hw     = database.HuaweiStorage{}
	ht     = database.HitachiStorage{}
	engine = "engine001"
	wwwn   = "fafokaoka"
)

func init() {
	orm, err := database.NewOrmerFromArgs(os.Args)
	if err != nil {
		fmt.Printf("%s,%s", os.Args, err)
		return
	}

	db = orm
}

func getScriptPath() string {
	gopath := os.Getenv("GOPATH")

	return filepath.Join(gopath, "src/github.com/docker/swarm/script")
}

func TestDefaultStores(t *testing.T) {
	path := getScriptPath()

	if db == nil {
		t.Skip("skip tests")
	}

	SetDefaultStores(path, db)

	ds := DefaultStores()

	out, err := ds.List()
	if err != nil || len(out) != 0 {
		t.Error(err, len(out))
	}

	{
		hws, err := ds.Add(hw.Vendor, hw.IPAddr, hw.Username, hw.Password, "", 0, 0, hw.HluStart, hw.HluEnd)
		if err != nil {
			t.Log(err)
		} else {
			hw.ID = hws.ID()

			_, err = ds.Get(hws.ID())
			if err != nil {
				t.Error(hws.ID(), err)
			}
		}
	}

	{
		hts, err := ds.Add(ht.Vendor, "", "", "", ht.AdminUnit, ht.LunStart, ht.LunEnd, ht.HluStart, ht.HluEnd)
		if err != nil {
			t.Log(err)
		} else {
			ht.ID = hts.ID()

			_, err = ds.Get(hts.ID())
			if err != nil {
				t.Error(hts.ID(), err)
			}
		}
	}

	out, err = ds.List()
	if err != nil || len(out) == 0 {
		t.Error(err, len(out))
	}
}

func TestStore(t *testing.T) {
	if db == nil {
		t.Skip("skip test")
	}

	ds := DefaultStores()

	s, err := ds.Get(hw.ID)
	if err == nil && s != nil {
		testStore(s, t)

		err = ds.Remove(s.ID())
		if err != nil {
			t.Error(err, s.ID())
		}

	} else {
		t.Log(err)
	}

	s, err = ds.Get(ht.ID)
	if err == nil && s != nil {
		testStore(s, t)

		err = ds.Remove(s.ID())
		if err != nil {
			t.Error(err, s.ID())
		}
	} else {
		t.Log(err)
	}
}

func testStore(s Store, t *testing.T) {
	err := s.ping()
	if err != nil {
		t.Error(err)
	}

	info, err := s.Info()
	if err != nil {
		t.Error(err, info)
	}

	ids := []string{"1", "2", "3"}
	for i := range ids {
		space, err := s.AddSpace(ids[i])
		if err != nil {
			t.Error(err, space)
		}

		err = s.DisableSpace(space.ID)
		if err != nil {
			t.Error(err)
		}

		err = s.EnableSpace(space.ID)
		if err != nil {
			t.Error(err)
		}

		defer func(id string) {
			err := s.removeSpace(id)
			if err != nil {
				t.Error(err)
			}
		}(ids[i])
	}

	err = s.AddHost(engine, wwwn)
	if err != nil {
		t.Error(err)
	}
	defer func() {
		err := s.DelHost(engine, wwwn)
		if err != nil {
			t.Error(err)
		}
	}()

	lun, lv, err := s.Alloc("volumeName0001", "unitID0001", "VGName001", 2<<30)
	if err != nil {
		t.Error(err)
	}

	defer func() {
		err := s.Recycle(lun.ID, 0)
		if err != nil {
			t.Error(err)
		}
	}()

	err = s.Mapping(engine, "VGName001", lun.ID, lv.UnitID)
	if err != nil {
		t.Error(err)
	}

	defer func() {
		err := s.DelMapping(lun)
		if err != nil {
			t.Error(err)
		}
	}()

	lun1, lv, err := s.Extend(lv, 1<<30)
	if err != nil {
		t.Error(err)
	}

	defer func() {
		err := s.Recycle(lun1.ID, 0)
		if err != nil {
			t.Error(err)
		}
	}()

	if lv.Size != 3<<30 {
		t.Error(lv.Size)
	}
}
