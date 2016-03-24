package database

import (
	"errors"

	"github.com/jmoiron/sqlx"
)

var (
	driverName     string
	dataSourceName string
	defaultDB      *sqlx.DB
)

// MustConnect connects to a database and panics on error.
func MustConnect(driverName, dataSourceName string) *sqlx.DB {
	defaultDB = sqlx.MustConnect(driverName, dataSourceName)

	driverName = driverName
	dataSourceName = dataSourceName

	return defaultDB
}

// Connect to a database and verify with a ping.
func Connect(driverName, dataSourceName string) (*sqlx.DB, error) {
	var err error

	defaultDB, err = sqlx.Connect(driverName, dataSourceName)

	if err == nil {
		driverName = driverName
		dataSourceName = dataSourceName
	}

	return defaultDB, err
}

func GetDB(ping bool) (*sqlx.DB, error) {
	var err error

	if defaultDB == nil {
		if driverName != "" && dataSourceName != "" {
			return Connect(driverName, dataSourceName)
		}

		return nil, errors.New("DB isnot open.")
	}

	if ping {
		err = defaultDB.Ping()
		if err != nil {
			return Connect(driverName, dataSourceName)
		}
	}

	return defaultDB, err
}
