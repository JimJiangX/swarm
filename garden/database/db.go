package database

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type Ormer interface {
	ServiceInterface
	ServiceInfoInterface
	ClusterInterface
	UnitInterface
	ContainerInterface
	ImageInterface
	NodeInterface

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
