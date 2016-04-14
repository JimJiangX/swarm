package database

import (
	"errors"
	"fmt"

	"github.com/codegangsta/cli"
	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
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

	source := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8&loc=Asia%%2FShanghai",
		user, password, host, port, name)

	_, err = Connect(driver, source)

	fmt.Println(driver, source, err)

	return err
}

// MustConnect connects to a database and panics on error.
func MustConnect(driver, source string) *sqlx.DB {
	defaultDB = sqlx.MustConnect(driver, source)

	driverName = driverName
	dbSource = source

	return defaultDB
}

// Connect to a database and verify with a ping.
func Connect(driver, source string) (*sqlx.DB, error) {
	var err error

	defaultDB, err = sqlx.Connect(driver, source)

	if err == nil {
		driverName = driverName
		dbSource = source
	}

	return defaultDB, err
}

func GetDB(ping bool) (*sqlx.DB, error) {
	var err error

	if defaultDB == nil {
		if driverName != "" && dbSource != "" {
			return Connect(driverName, dbSource)
		}

		return nil, errors.New("DB isnot open.")
	}

	if ping {
		err = defaultDB.Ping()
		if err != nil {
			return Connect(driverName, dbSource)
		}
	}

	return defaultDB, err
}
