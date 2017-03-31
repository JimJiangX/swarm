package database

import (
	"errors"
	"log"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	driverName     string
	dbSource       string
	dbMaxIdleConns int
	ormer          Ormer
	db             *dbBase
)

func init() {
	dbSource = "root:root@tcp(192.168.4.130:3306)/mgm?parseTime=true&charset=utf8&loc=Asia%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'"
	driverName = "mysql"
	dbMaxIdleConns = 8
	orm, err := NewOrmer(driverName, dbSource, "tbl", dbMaxIdleConns)
	if err != nil {
		log.Printf("%+v", err)
		return
	}

	ormer = orm
	db = orm.(*dbBase)
}

func TestTxFrame(t *testing.T) {
	if ormer == nil {
		t.Skip("orm:db is required")
	}

	err := ormer.TxFrame(
		func(tx *sqlx.Tx) error {
			return nil
		})

	if err != nil {
		t.Errorf("Unexpected,want nil error but got %s", err)
	}

	var errFake = errors.New("fake error")
	err = ormer.TxFrame(
		func(tx *sqlx.Tx) error {
			return errFake
		})

	if err != errFake {
		t.Errorf("Unexpected,want %s but got %v", errFake, err)
	}
}
