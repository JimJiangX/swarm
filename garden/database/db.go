package database

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

//var (
//	driverName     string
//	dbSource       string
//	dbMaxIdleConns int
//	defaultDB      *sqlx.DB

//	// FlDBDriver DB driver
//	FlDBDriver = cli.StringFlag{
//		Name:  "dbDriver",
//		Value: "mysql",
//		Usage: "database driver name",
//	}
//	// FlDBName DB name
//	FlDBName = cli.StringFlag{
//		Name:  "dbName",
//		Usage: "database name",
//	}
//	// FlDBAuth DB auth for db login
//	FlDBAuth = cli.StringFlag{
//		Name:  "dbAuth",
//		Usage: "auth for login database",
//	}
//	// FlDBHost DB host address
//	FlDBHost = cli.StringFlag{
//		Name:  "dbHost",
//		Value: "127.0.0.1",
//		Usage: "connection to database host addr",
//	}
//	// FlDBPort DB port
//	FlDBPort = cli.IntFlag{
//		Name:  "dbPort",
//		Value: 3306,
//		Usage: "connection to database port",
//	}
//	// FlDBMaxOpenConns DB max open conns
//	FlDBMaxOpenConns = cli.IntFlag{
//		Name:  "dbMaxOpen",
//		Value: 20,
//		Usage: "max open connection of DB,<= 0 means unlimited",
//	}
//)

//// SetupDB opens *sql.DB
//func SetupDB(c *cli.Context) error {
//	auth := c.String("dbAuth")
//	user, password, err := utils.Base64Decode(auth)
//	if err != nil {
//		return err
//	}

//	driver := c.String("dbDriver")
//	name := c.String("dbName")

//	host := c.String("dbHost")
//	port := c.Int("dbPort")
//	maxIdle := c.Int("dbMaxOpen")

//	source := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8&loc=Asia%%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'",
//		user, password, host, port, name)

//	dbMaxIdleConns = maxIdle
//	driverName = driver
//	dbSource = source

//	defaultDB, err = Connect(driver, source, maxIdle)

//	return err
//}

type Ormer interface {
	ServiceOrmer

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

func (db dbBase) txFrame(do func(tx *sqlx.Tx) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return errors.Wrap(err, "Tx Begin")
	}

	defer tx.Rollback()

	err = do(tx)
	if err == nil {
		err = errors.Wrap(tx.Commit(), "Tx Commit")
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
