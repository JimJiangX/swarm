package database

import (
	"runtime/debug"
	"testing"
)

func init() {
	dbSource = "root:111111@tcp(192.168.2.121:3306)/DBaaS_test?parseTime=true&charset=utf8&loc=Asia%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'"
	driverName = "mysql"
	dbMaxIdleConns = 8
}

func TestConnect(t *testing.T) {
	db, err := Connect(driverName, dbSource, dbMaxIdleConns)
	if db == nil || err != nil {
		t.Fatal(err)
	}

	if dbSource == "" || driverName != "mysql" {
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

	db := MustConnect(driverName, dbSource, dbMaxIdleConns)
	if db == nil {
		t.FailNow()
	}

	if dbSource == "" || driverName != "mysql" ||
		defaultDB == nil || defaultDB != db {
		t.Fatal("Unexpected")
	}
}

func TestGetDB(t *testing.T) {
	db, err := getDB(false)
	if err != nil || db == nil {
		t.Error("Unexpected", err)
	}

	db.Close()

	db, err = getDB(false)
	if err != nil || db == nil {
		t.Error("Unexpected", err)
	}

	if err = db.Ping(); err != nil {
		t.Log("Expected", err)
	}

	db, err = getDB(true)
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
