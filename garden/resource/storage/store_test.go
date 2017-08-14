package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/swarm/garden/database"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

var (
	db database.Ormer

	ht = database.SANStorage{
		Vendor:    "HITACHI",
		Version:   "",
		AdminUnit: "100",
		LunStart:  1000,
		LunEnd:    1200,
		HluStart:  500,
		HluEnd:    600,
	}
	engine = "2QAC:RHRO:QZRZ:QNJV:TXT6:NV2S:WNEO:W47V:6DNL:5EO2:KEPB:LRWN"
	wwwn   = []string{"10000090fae3a561", "10000090fae3b0c4", "10000090fae3b0c5", "10000090fae3a560"}
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
		t.Errorf("%+v %d", err, len(out))
	}

	if ht.Vendor != "" {
		hts, err := ds.Add(ht.Vendor, ht.Version, "", "", "", ht.AdminUnit, ht.LunStart, ht.LunEnd, ht.HluStart, ht.HluEnd)
		if err != nil {
			t.Errorf("%+v", err)
		} else {
			ht.ID = hts.ID()

			_, err = ds.Get(hts.ID())
			if err != nil {
				t.Errorf("%s:%+v", hts.ID(), err)
			}
		}
	}

	out, err = ds.List()
	if err != nil || len(out) == 0 {
		t.Errorf("%+v %d", err, len(out))
	}
}

func TestStore(t *testing.T) {
	if db == nil {
		t.Skip("skip test")
	}

	ds := DefaultStores()

	s, err := ds.Get(ht.ID)
	if err == nil && s != nil {
		testStore(s, t)

		err = ds.Remove(s.ID())
		if err != nil {
			t.Errorf("%s:%+v ", s.ID(), err)
		}
	} else {
		t.Logf("%+v", err)
	}
}

func testStore(s Store, t *testing.T) {
	err := s.ping()
	if err != nil {
		t.Errorf("%+v", err)
	}

	info, err := s.Info()
	if err != nil {
		t.Errorf("%+v\n%v", err, info)
	}

	ids := []string{"1-1", "1-2", "1-3"}
	for i := range ids {
		defer func(rg string) {
			err := s.removeSpace(rg)
			if err != nil {
				t.Errorf("%+v", err)
			}
		}(ids[i])

		space, err := s.AddSpace(ids[i])
		if err != nil {
			t.Errorf("%+v", err)
			continue
		}

		err = s.DisableSpace(space.ID)
		if err != nil {
			t.Errorf("%+v", err)
		}

		err = s.EnableSpace(space.ID)
		if err != nil {
			t.Errorf("%+v", err)
		}
	}

	err = testAlloc(s)
	if err != nil {
		t.Errorf("%+v", err)
	}
}

func testAlloc(s Store) (err error) {
	err = s.AddHost(engine, wwwn...)
	if err != nil {
		return err
	}
	defer func() {
		_err := s.DelHost(engine, wwwn...)
		if _err != nil {
			if err == nil {
				err = _err
			} else {
				err = fmt.Errorf("%+v\n,del host,%+v", err, _err)
			}
		}
	}()

	lun, lv, err := s.Alloc("volumeName0001", "unitID0001", "VGName001", 2<<30)
	if err != nil {
		return err
	}

	defer func() {
		_err := s.Recycle(lun.ID, 0)
		if _err != nil {
			if err == nil {
				err = _err
			} else {
				err = fmt.Errorf("%+v\n,recycle lun %d,%+v", err, lun.HostLunID, _err)
			}
		}
	}()

	err = s.Mapping(engine, "VGName001", lun.ID, lv.UnitID)
	if err != nil {
		return err
	}

	defer func() {
		_err := s.DelMapping(lun)
		if _err != nil {
			if err == nil {
				err = _err
			} else {
				err = fmt.Errorf("%+v\n,del mapping %d,%+v", err, lun.HostLunID, _err)
			}
		}
	}()

	lun1, lv, err := s.Extend(lv, 1<<30)
	if err != nil {
		return err
	}

	defer func() {
		_err := s.Recycle(lun1.ID, 0)
		if _err != nil {
			if err == nil {
				err = _err
			} else {
				err = fmt.Errorf("%+v\n,recycle lun %d,%+v", err, lun1.HostLunID, _err)
			}
		}
	}()

	defer func() {
		_err := s.DelMapping(lun1)
		if _err != nil {
			if err == nil {
				err = _err
			} else {
				err = fmt.Errorf("%+v\n,del mapping %d,%+v", err, lun1.HostLunID, _err)
			}
		}
	}()

	if lv.Size != 3<<30 {
		return errors.Errorf("expect volume size extend to 3G,but got %d", lv.Size)
	}

	return nil
}
