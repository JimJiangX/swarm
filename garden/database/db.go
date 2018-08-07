package database

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/swarm/garden/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type Ormer interface {
	ServiceIface
	ServiceInfoIface
	//	ClusterIface
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

	MarkRunningTasks() error
	TxFrame(do func(tx *sqlx.Tx) error) error
}

type dbBase struct {
	prefix string
	*sqlx.DB
}

// NewOrmer connect to a database and verify with Ping.
func NewOrmer(driver, source, prefix string, idle, open int) (Ormer, error) {
	db, err := sqlx.Connect(driver, source)
	if err != nil {
		if db != nil {
			db.Close()
		}

		return nil, errors.Wrap(err, "DB connection")
	}

	db.SetMaxOpenConns(open)
	db.SetMaxIdleConns(idle)
	db.SetConnMaxLifetime(time.Hour)

	return &dbBase{DB: db, prefix: prefix}, nil
}

// NewOrmerFromArgs args example:
// go test -v -args dbHost=192.168.4.130 dbPort=3306 dbDriver=mysql dbName=mgm  dbAuth=cm9vdDpyb290 dbTablePrefix=tbl dbMaxIdle=5 dbMaxOpen=10
func NewOrmerFromArgs(args []string) (Ormer, error) {
	var auth, user, password, driver, name, host, port, prefix string
	var maxIdle, maxOpen int

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

			case "dbMaxOpen":
				if val == "" {
					val = "0"
				}
				maxOpen, _ = strconv.Atoi(val)

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

	if maxOpen == 0 {
		maxOpen = 2 * maxIdle

	}

	o, err := NewOrmer(driver, source, prefix, maxIdle, maxOpen)

	return o, err
}

func (db dbBase) txFrame(do func(tx *sqlx.Tx) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return errors.Wrap(err, "Tx Begin")
	}

	err = do(tx)
	if err == nil {
		return errors.Wrap(tx.Commit(), "Tx Commit")
	}

	if _err := tx.Rollback(); _err != nil {
		return fmt.Errorf("%s\n%+v", _err, err)
	}

	return err
}

// TxFrame is a frame for Tx functions.
func (db dbBase) TxFrame(do func(tx *sqlx.Tx) error) error {
	return db.txFrame(do)
}

func IsNotFound(err error) bool {
	return errors.Cause(err) == sql.ErrNoRows
}
