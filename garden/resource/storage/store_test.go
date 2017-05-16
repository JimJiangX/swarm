package storage

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/swarm/garden/database"
	_ "github.com/go-sql-driver/mysql"
)

var (
	db database.Ormer

	hw = database.HuaweiStorage{}
	ht = database.HitachiStorage{}
)

func init() {
	dbSource := "root:root@tcp(192.168.4.130:3306)/mgm?parseTime=true&charset=utf8&loc=Asia%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'"
	driverName := "mysql"
	dbMaxIdleConns := 8

	var err error

	db, err = database.NewOrmer(driverName, dbSource, "tbl", dbMaxIdleConns)
	if err != nil {
		log.Printf("%+v", err)
		return
	}
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
