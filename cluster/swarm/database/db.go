package database

import (
	"fmt"

	"github.com/codegangsta/cli"
	"github.com/docker/swarm/utils"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

var (
	driverName     string
	dbSource       string
	dbMaxIdleConns int
	defaultDB      *sqlx.DB

	// FlDBDriver DB driver
	FlDBDriver = cli.StringFlag{
		Name:  "dbDriver",
		Value: "mysql",
		Usage: "database driver name",
	}
	// FlDBName DB name
	FlDBName = cli.StringFlag{
		Name:  "dbName",
		Usage: "database name",
	}
	// FlDBAuth DB auth for db login
	FlDBAuth = cli.StringFlag{
		Name:  "dbAuth",
		Usage: "auth for login database",
	}
	// FlDBHost DB host address
	FlDBHost = cli.StringFlag{
		Name:  "dbHost",
		Value: "127.0.0.1",
		Usage: "connection to database host addr",
	}
	// FlDBPort DB port
	FlDBPort = cli.IntFlag{
		Name:  "dbPort",
		Value: 3306,
		Usage: "connection to database port",
	}
	// FlDBMaxOpenConns DB max open conns
	FlDBMaxOpenConns = cli.IntFlag{
		Name:  "dbMaxOpen",
		Value: 20,
		Usage: "max open connection of DB,<= 0 means unlimited",
	}
)

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
	maxIdle := c.Int("dbMaxOpen")

	source := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8&loc=Asia%%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'",
		user, password, host, port, name)

	dbMaxIdleConns = maxIdle
	driverName = driver
	dbSource = source

	defaultDB, err = Connect(driver, source, maxIdle)

	return err
}

// MustConnect connects to a database and panics on error.
func MustConnect(driver, source string, maxOpen int) *sqlx.DB {
	defaultDB = sqlx.MustConnect(driver, source)
	defaultDB.SetMaxIdleConns(maxOpen)

	return defaultDB
}

// Connect to a database and verify with Ping.
func Connect(driver, source string, max int) (*sqlx.DB, error) {
	db, err := sqlx.Connect(driver, source)
	if err != nil {
		if db != nil {
			db.Close()
		}

		return nil, errors.Wrap(err, "DB connection")
	}

	db.SetMaxIdleConns(max)

	return db, nil
}

// GetDB returns *sqlx.DB for DB operation.
// if defaultDB is non-nil,use defaultDB.
// if ping is true calls DB.Ping.
// open a new DB if error happened.
func getDB(ping bool) (*sqlx.DB, error) {
	if defaultDB != nil {

		if !ping {
			return defaultDB, nil
		}

		if err := defaultDB.Ping(); err == nil {
			return defaultDB, nil
		}

		defaultDB.Close()
	}

	db, err := Connect(driverName, dbSource, dbMaxIdleConns)
	defaultDB = db

	return db, err
}

// GetTX begin a new Tx.
func GetTX() (*sqlx.Tx, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	tx, err := db.Beginx()

	return tx, errors.Wrap(err, "TX begin")
}
