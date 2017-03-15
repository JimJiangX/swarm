package database

import (
	"errors"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	driverName     string
	dbSource       string
	dbMaxIdleConns int
	ormer          Ormer
)

func init1() {
	var err error
	dbSource = "root:111111@tcp(192.168.2.121:3306)/DBaaS_test?parseTime=true&charset=utf8&loc=Asia%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'"
	driverName = "mysql"
	dbMaxIdleConns = 8
	ormer, err = NewOrmer(driverName, dbSource, "tb", dbMaxIdleConns)
	if err != nil {
		panic(err)
	}
}

func TestTxFrame(t *testing.T) {
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
