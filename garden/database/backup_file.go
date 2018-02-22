package database

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type BackupFileIface interface {
	InsertBackupFileWithTask(bf BackupFile, t Task) error

	GetBackupFile(id string) (BackupFile, error)

	ListBackupFiles() ([]BackupFile, error)

	ListBackupFilesByTag(tag string) ([]BackupFile, error)

	ListBackupFilesByService(nameOrID string) ([]BackupFile, error)

	DelBackupFiles(files []BackupFile) error
}

// BackupFile is table _backup_files structure,correspod with backup files
type BackupFile struct {
	ID         string    `db:"id" json:"id"`
	TaskID     string    `db:"task_id" json:"task_id"`
	UnitID     string    `db:"unit_id" json:"unit_id"`
	Type       string    `db:"type" json:"type"` // full or incremental or tables
	Tables     string    `db:"tables" json:"tables"`
	Path       string    `db:"path" json:"path"`
	Remark     string    `db:"remark" json:"remark"`
	Tag        string    `db:"tag" json:"tag"`
	SizeByte   int       `db:"size" json:"size"`
	Retention  time.Time `db:"retention" json:"retention"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	FinishedAt time.Time `db:"finished_at" json:"finished_at"`
}

func (db dbBase) backupFileTable() string {
	return db.prefix + "_backup_files"
}

// ListBackupFiles return all BackupFile
func (db dbBase) ListBackupFiles() ([]BackupFile, error) {
	var (
		out   []BackupFile
		query = "SELECT id,task_id,unit_id,type,tables,path,remark,tag,size,retention,created_at,finished_at FROM " + db.backupFileTable() + " ORDER BY finished_at DESC"
	)

	err := db.Select(&out, query)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []BackupFile")
}

// ListBackupFilesByTag return all []BackupFile by tag
func (db dbBase) ListBackupFilesByTag(tag string) ([]BackupFile, error) {
	var (
		out   []BackupFile
		query = "SELECT id,task_id,unit_id,type,tables,path,remark,tag,size,retention,created_at,finished_at FROM " + db.backupFileTable() + " WHERE tag=? ORDER BY finished_at DESC"
	)

	err := db.Select(&out, query, tag)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []BackupFile")
}

// ListBackupFilesByService returns []BackupFile select by name or ID
func (db dbBase) ListBackupFilesByService(service string) ([]BackupFile, error) {
	var (
		units []string
		files []BackupFile
		query = "SELECT id FROM " + db.unitTable() + " WHERE service_id=?"
	)

	err := db.Select(&units, query, service)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Wrapf(err, "not found Units by service_id='%s'", service)
	}

	if len(units) == 0 {
		return nil, nil
	}

	query = "SELECT id,task_id,unit_id,type,tables,path,remark,tag,size,retention,created_at,finished_at FROM " + db.backupFileTable() + " WHERE unit_id IN (?);"

	query, args, err := sqlx.In(query, units)
	if err != nil {
		return nil, errors.Wrap(err, "select BackupFile by UnitIDs")
	}

	err = db.Select(&files, query, args...)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Wrapf(err, "not found BackupFile by unit_id='%s'", units)
	}

	return files, nil
}

// GetBackupFile returns BackupFile select by ID
func (db dbBase) GetBackupFile(id string) (BackupFile, error) {
	var row BackupFile

	query := "SELECT id,task_id,unit_id,type,tables,path,remark,tag,size,retention,created_at,finished_at FROM " + db.backupFileTable() + " WHERE id=?"

	err := db.Get(&row, query, id)

	return row, errors.Wrap(err, "get Backup File by ID")
}

func (db dbBase) txInsertBackupFile(tx *sqlx.Tx, bf BackupFile) error {
	query := "INSERT INTO " + db.backupFileTable() + " (id,task_id,unit_id,type,tables,path,remark,tag,size,retention,created_at,finished_at) VALUES (:id,:task_id,:unit_id,:type,:tables,:path,:remark,:tag,:size,:retention,:created_at,:finished_at)"

	_, err := tx.NamedExec(query, bf)

	return errors.Wrap(err, "Tx insert BackupFile")
}

func (db dbBase) InsertBackupFileWithTask(bf BackupFile, t Task) error {
	do := func(tx *sqlx.Tx) error {
		err := db.txInsertBackupFile(tx, bf)
		if err != nil {
			return err
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
