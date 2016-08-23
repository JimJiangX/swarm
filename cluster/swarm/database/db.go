package database

import (
	"fmt"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/docker/swarm/utils"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

var (
	driverName string
	dbSource   string
	defaultDB  *sqlx.DB

	FlDBDriver = cli.StringFlag{
		Name:  "dbDriver",
		Value: "mysql",
		Usage: "database driver name",
	}
	FlDBName = cli.StringFlag{
		Name:  "dbName",
		Usage: "database name",
	}
	FlDBAuth = cli.StringFlag{
		Name:  "dbAuth",
		Usage: "auth for login database",
	}
	FlDBHost = cli.StringFlag{
		Name:  "dbHost",
		Value: "127.0.0.1",
		Usage: "connection to database host addr",
	}
	FlDBPort = cli.IntFlag{
		Name:  "dbPort",
		Value: 3306,
		Usage: "connection to database port",
	}
)

var (
	noRowsFoundMsg     = "no rows in result set"
	connectioneRefused = "connection refused"
	ErrNoRowsFound     = errors.New("not found object")
	ErrDisconnected    = errors.New("DB disconnected")
)

// CheckError check error if error is ErrNoRowsFound or ErrDisconnected
func CheckError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	if strings.Contains(msg, noRowsFoundMsg) {
		return ErrNoRowsFound
	}

	if strings.Contains(msg, connectioneRefused) {
		return ErrDisconnected
	}

	return err
}

// SetupDB opens *sql.DB
func SetupDB(c *cli.Context) error {
	auth := c.String("dbAuth")
	user, password, err := utils.Base64Decode(auth)
	if err != nil {
		return err
	}

	driver := c.String("dbDriver")
	name := c.String("dbName")

	host := c.String("dbHost")
	port := c.Int("dbPort")

	source := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8&loc=Asia%%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'",
		user, password, host, port, name)

	_, err = Connect(driver, source)

	return err
}

// MustConnect connects to a database and panics on error.
func MustConnect(driver, source string) *sqlx.DB {
	defaultDB = sqlx.MustConnect(driver, source)

	driverName = driver
	dbSource = source

	return defaultDB
}

// Connect to a database and verify with Ping.
func Connect(driver, source string) (*sqlx.DB, error) {
	db, err := sqlx.Connect(driver, source)
	if err != nil {
		if db != nil {
			db.Close()
		}

		return nil, errors.Wrap(err, "DB connection")
	}

	driverName = driver
	dbSource = source
	defaultDB = db

	return db, nil
}

// GetDB returns *sqlx.DB for DB operation.
// if defaultDB is non-nil,use defaultDB.
// if ping is true calls DB.Ping.
// open a new DB if error happened.
func GetDB(ping bool) (*sqlx.DB, error) {
	if defaultDB != nil {

		if !ping {
			return defaultDB, nil
		}

		if err := defaultDB.Ping(); err == nil {
			return defaultDB, nil

		} else {
			defaultDB.Close()
		}
	}

	return Connect(driverName, dbSource)
}

// GetTX begin a new Tx.
func GetTX() (*sqlx.Tx, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	tx, err := db.Beginx()
	if err != nil {
		db, err = Connect(driverName, dbSource)
		if err != nil {
			return nil, err
		}

		tx, err = db.Beginx()
		if err != nil {
			return nil, errors.Wrap(err, "TX begin")
		}
	}

	return tx, nil
}
