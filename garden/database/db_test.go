package database

import (
	"errors"
	"fmt"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	driverName     string
	dbSource       string
	dbMaxIdleConns int

	ormer Ormer
	db    *dbBase
)

func init() {
	orm, err := NewOrmerFromArgs(os.Args)
	if err != nil {
		fmt.Printf("%s,%s", os.Args, err)
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
