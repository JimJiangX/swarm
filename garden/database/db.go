package database

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type Ormer interface {
	ServiceIface
	ServiceInfoIface
	ClusterIface
	UnitIface
	ContainerIface
	ImageIface
	NodeIface
	StorageIface
	BackupFileIface

	SysConfigOrmer
	NetworkingOrmer
	TaskOrmer
	VolumeOrmer

	TxFrame(do func(tx *sqlx.Tx) error) error
}

type dbBase struct {
	prefix string
	*sqlx.DB
}

// NewOrmer connect to a database and verify with Ping.
func NewOrmer(driver, source, prefix string, max int) (Ormer, error) {
	db, err := sqlx.Connect(driver, source)
	if err != nil {
		if db != nil {
			db.Close()
		}

		return nil, errors.Wrap(err, "DB connection")
	}

	db.SetMaxIdleConns(max)

	return &dbBase{DB: db, prefix: prefix}, nil
}

// NewOrmerFromArgs args example:
// go test -v -args dbHost=192.168.4.130 dbPort=3306 dbDriver=mysql dbName=mgm  dbAuth=cm9vdDpyb290 dbTablePrefix=tbl dbMaxIdle=10
func NewOrmerFromArgs(args []string) (Ormer, error) {
	var auth, user, password, driver, name, host, port, prefix string
	var maxIdle int

	for i := range args {

		list := strings.Split(args[i], " ")

		for l := range list {
			parts := strings.SplitN(list[l], "=", 2)
			if len(parts) != 2 {
				continue
			}

			val := strings.TrimSpace(parts[1])

			switch strings.TrimSpace(parts[0]) {
			case "dbAuth":
				auth = val

			case "user":
				user = val

			case "password":
				password = val

			case "dbDriver":
				driver = val

			case "dbName":
				name = val

			case "dbHost":
				host = val

			case "dbPort":
				port = val
				if port == "" {
					port = "3306"
				}

			case "dbMaxIdle":
				if val == "" {
					val = "0"
				}
				maxIdle, _ = strconv.Atoi(val)

			case "dbTablePrefix":
				prefix = val

			default:
			}
		}
	}

	if auth != "" && user == "" {
		user, password, _ = utils.Base64Decode(auth)
	}

	if user == "" || driver == "" || name == "" || host == "" {
		return nil, errors.New("db config is required")
	}

	source := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8&loc=Asia%%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'",
		user, password, host, port, name)

	o, err := NewOrmer(driver, source, prefix, maxIdle)

	return o, err
}

func (db dbBase) txFrame(do func(tx *sqlx.Tx) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return errors.Wrap(err, "Tx Begin")
	}

	defer tx.Rollback()

	err = do(tx)
	if err == nil {
		err = tx.Commit()
		if err != nil {
			return errors.Wrap(err, "Tx Commit")
		}
	}

	return err
}

// TxFrame is a frame for Tx functions.
func (db dbBase) TxFrame(do func(tx *sqlx.Tx) error) error {
	return db.txFrame(do)
}

func IsNotFound(err error) bool {
	_err := errors.Cause(err)

	return _err == sql.ErrNoRows
}
