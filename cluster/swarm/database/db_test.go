package database

import (
	"runtime/debug"
	"testing"
)

func init() {
	dbSource = "root:111111@tcp(127.0.0.1:3306)/DBaaS?parseTime=Truecharset=utf8&loc=Asia%2FShanghai"
	driverName = "mysql"
}

func TestConnect(t *testing.T) {
	db, err := Connect(driverName, dbSource)
	if db == nil || err != nil {
		t.Fatal(err)
	}

	if dbSource == "" || driverName != "mysql" || defaultDB != db {
		t.Fatal("Unexpected")
	}
}

func TestMustConnect(t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			debug.PrintStack()
			t.Fatal(err)
		}
	}()
	db := MustConnect(driverName, dbSource)
	if db == nil {
		t.FailNow()
	}

	if dbSource == "" || driverName != "mysql" || defaultDB != db {
		t.Fatal("Unexpected")
	}
}

func TestGetDB(t *testing.T) {
	db, err := GetDB(false)
	if err != nil || db == nil {
		t.Fatal(err, db)
	}

	db, err = GetDB(true)
	if err != nil || db == nil {
		t.Fatal("With Ping", err, db)
	}
}

func TestGetTX(t *testing.T) {
	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}
}
