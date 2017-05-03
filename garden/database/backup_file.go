package database

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type BackupFileIface interface {
	InsertBackupFileWithTask(bf BackupFile, t Task) error

	ListBackupFilesByService(nameOrID string) (Service, []BackupFile, error)

	DelBackupFiles(files []BackupFile) error
}

// BackupFile is table _backup_files structure,correspod with backup files
type BackupFile struct {
	ID         string    `db:"id"`
	TaskID     string    `db:"task_id"`
	UnitID     string    `db:"unit_id"`
	Type       string    `db:"type"` // full or incremental
	Path       string    `db:"path"`
	SizeByte   int       `db:"size"`
	Retention  time.Time `db:"retention"`
	CreatedAt  time.Time `db:"created_at"`
	FinishedAt time.Time `db:"finished_at"`
}

func (db dbBase) backupFileTable() string {
	return db.prefix + "_backup_files"
}

// ListBackupFiles return all BackupFile
func (db dbBase) ListBackupFiles() ([]BackupFile, error) {
	var (
		out   []BackupFile
		query = "SELECT id,task_id,unit_id,type,path,size,retention,created_at,finished_at FROM " + db.backupFileTable()
	)

	err := db.Select(&out, query)
	if err == nil {
		return out, nil
	} else if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, errors.Wrap(err, "list []BackupFile")
}

// ListBackupFilesByService returns Service and []BackupFile select by name or ID
func (db dbBase) ListBackupFilesByService(nameOrID string) (Service, []BackupFile, error) {
	service, err := db.GetService(nameOrID)
	if err != nil {
		return service, nil, errors.Wrapf(err, "not found Service by '%s'", nameOrID)
	}

	var (
		units []string
		files []BackupFile
		query = "SELECT id FROM " + db.unitTable() + " WHERE service_id=?"
	)

	err = db.Select(&units, query, service.ID)
	if err != nil && err != sql.ErrNoRows {
		return service, nil, errors.Wrapf(err, "not found Units by service_id='%s'", service.ID)
	}

	if len(units) == 0 {
		return service, nil, nil
	}

	query = "SELECT id,task_id,unit_id,type,path,size,retention,created_at,finished_at FROM " + db.backupFileTable() + " WHERE unit_id IN (?);"

	query, args, err := sqlx.In(query, units)
	if err != nil {
		return service, nil, errors.Wrap(err, "select BackupFile by UnitIDs")
	}

	err = db.Select(&files, query, args...)
	if err != nil && err != sql.ErrNoRows {
		return service, nil, errors.Wrapf(err, "not found BackupFile by unit_id='%s'", units)
	}

	return service, files, nil
}

// GetBackupFile returns BackupFile select by ID
func (db dbBase) GetBackupFile(id string) (BackupFile, error) {
	var (
		row   BackupFile
		query = "SELECT id,task_id,unit_id,type,path,size,retention,created_at,finished_at FROM " + db.backupFileTable() + " WHERE id=?"
	)
	err := db.Get(&row, query, id)
	if err == nil {
		return row, nil
	}

	return row, errors.Wrap(err, "get Backup File by ID")
}

func (db dbBase) txInsertBackupFile(tx *sqlx.Tx, bf BackupFile) error {
	query := "INSERT INTO " + db.backupFileTable() + " (id,task_id,unit_id,type,path,size,retention,created_at,finished_at) VALUES (:id,:task_id,:unit_id,:type,:path,:size,:retention,:created_at,:finished_at)"

	_, err := tx.NamedExec(query, bf)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx insert BackupFile")
}

func (db dbBase) InsertBackupFileWithTask(bf BackupFile, t Task) error {
	do := func(tx *sqlx.Tx) error {
		err := db.txInsertBackupFile(tx, bf)
		if err != nil {
			return nil
		}

		return db.txSetTask(tx, t)
	}

	return db.txFrame(do)
}

func (db dbBase) DelBackupFiles(files []BackupFile) error {
	do := func(tx *sqlx.Tx) error {
		query := "DELETE FROM " + db.backupFileTable() + " WHERE id=?"

		stmt, err := tx.Preparex(query)
		if err != nil {
			return errors.WithStack(err)
		}

		for i := range files {
			_, err := stmt.Exec(files[i].ID)
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "tx delete []BackupFile")
			}
		}

		stmt.Close()

		return nil
	}

	return db.txFrame(do)
}
