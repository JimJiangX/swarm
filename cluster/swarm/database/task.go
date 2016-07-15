package database

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
)

const insertTaskQuery = "INSERT INTO tb_task (id,related,link_to,description,labels,errors,timeout,status,created_at,finished_at) VALUES (:id,:related,:link_to,:description,:labels,:errors,:timeout,:status,:created_at,:finished_at)"

type Task struct {
	ID string `db:"id"`
	//	Name        string        `db:"name"`
	Related     string    `db:"related"`
	Linkto      string    `db:"link_to"`
	Description string    `db:"description"`
	Labels      string    `db:"labels"`
	Errors      string    `db:"errors"`
	Timeout     int       `db:"timeout"` // s
	Status      int32     `db:"status"`
	CreatedAt   time.Time `db:"created_at"`
	FinishedAt  time.Time `db:"finished_at"`
}

func (t Task) TableName() string {
	return "tb_task"
}

const insertBackupFileQuery = "INSERT INTO tb_backup_files (id,task_id,strategy_id,unit_id,type,path,size,retention,created_at,finished_at) VALUES (:id,:task_id,:strategy_id,:unit_id,:type,:path,:size,:retention,:created_at,:finished_at)"

type BackupFile struct {
	ID         string    `db:"id"`
	TaskID     string    `db:"task_id"`
	StrategyID string    `db:"strategy_id"`
	UnitID     string    `db:"unit_id"`
	Type       string    `db:"type"` // full or incremental
	Path       string    `db:"path"`
	SizeByte   int       `db:"size"`
	Retention  time.Time `db:"retention"`
	CreatedAt  time.Time `db:"created_at"`
	FinishedAt time.Time `db:"finished_at"`
}

func (bf BackupFile) TableName() string {
	return "tb_backup_files"
}

func ListBackupFilesByService(nameOrID string) ([]BackupFile, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var service string
	err = db.Get(&service, "SELECT id FROM tb_service WHERE id=? OR name=?", nameOrID, nameOrID)

	units := make([]string, 0, 5)
	err = db.Select(&units, "SELECT id FROM tb_unit WHERE service_id=?", service)
	if err != nil {
		return nil, err
	}

	query, args, err := sqlx.In("SELECT * FROM tb_backup_files WHERE unit_id IN (?);", units)
	if err != nil {
		return nil, err
	}

	files := make([]BackupFile, 0, 10)
	err = db.Select(&files, query, args...)
	if err != nil {
		return nil, err
	}

	return files, nil
}

func GetBackupFile(id string) (BackupFile, error) {
	db, err := GetDB(true)
	if err != nil {
		return BackupFile{}, err
	}

	row := BackupFile{}
	err = db.Get(&row, "SELECT * FROM tb_backup_files WHERE id=?", id)

	return row, err
}

func txInsertBackupFile(tx *sqlx.Tx, bf BackupFile) error {
	_, err := tx.NamedExec(insertBackupFileQuery, &bf)

	return err
}

func NewTask(relate, linkto, des string, labels []string, timeout int) Task {
	return Task{
		ID:          utils.Generate64UUID(),
		Related:     relate,
		Linkto:      linkto,
		Description: des,
		Labels:      strings.Join(labels, "&;&"),
		Timeout:     timeout,
		Status:      0,
		CreatedAt:   time.Now(),
	}
}

func (task Task) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertTaskQuery, &task)

	return err
}

func TxInsertTask(tx *sqlx.Tx, t Task) error {
	_, err := tx.NamedExec(insertTaskQuery, &t)

	return err
}

func TxInsertMultiTask(tx *sqlx.Tx, tasks []*Task) error {
	stmt, err := tx.PrepareNamed(insertTaskQuery)
	if err != nil {
		return err
	}

	for i := range tasks {
		if tasks[i] == nil {
			continue
		}

		_, err = stmt.Exec(tasks[i])
		if err != nil {
			stmt.Close()

			return err
		}
	}

	return stmt.Close()
}

func TxUpdateTaskStatus(tx *sqlx.Tx, t *Task, state int, finish time.Time, msg string) error {
	query := "UPDATE tb_task SET status=?,finished_at=?,errors=? WHERE id=?"

	if finish.IsZero() {
		query = "UPDATE tb_task SET status=?,errors=? WHERE id=?"

		_, err := tx.Exec(query, state, msg, t.ID)
		if err != nil {
			return err
		}

		atomic.StoreInt32(&t.Status, int32(state))

		return nil
	}

	_, err := tx.Exec(query, state, finish, msg, t.ID)
	if err != nil {
		return err
	}

	atomic.StoreInt32(&t.Status, int32(state))
	t.FinishedAt = finish

	return nil
}

func TxBackupTaskDone(task *Task, state int, backupFile BackupFile) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txInsertBackupFile(tx, backupFile)
	if err != nil {
		return err
	}

	err = TxUpdateTaskStatus(tx, task, state, time.Now(), "")
	if err != nil {
		return err
	}

	return tx.Commit()
}

func UpdateTaskStatus(task *Task, state int32, finishAt time.Time, msg string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_task SET status=?,finished_at=?,errors=? WHERE id=?"

	if finishAt.IsZero() {
		query = "UPDATE tb_task SET status=?,errors=? WHERE id=?"

		_, err := db.Exec(query, state, msg, task.ID)
		if err != nil {
			return err
		}

		atomic.StoreInt32(&task.Status, state)

		return nil
	}

	_, err = db.Exec(query, state, finishAt, msg, task.ID)
	if err != nil {
		return err
	}

	atomic.StoreInt32(&task.Status, state)
	task.FinishedAt = finishAt

	return nil
}

func (task *Task) UpdateStatus(status int, msg string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	now := time.Now()
	query := "UPDATE tb_task SET status=?,finished_at=?,errors=? WHERE id=?"

	_, err = db.Exec(query, status, now, msg, task.ID)
	if err != nil {
		return err
	}

	atomic.StoreInt32(&task.Status, int32(status))
	task.FinishedAt = now

	return nil
}

func GetTask(id string) (*Task, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	t := &Task{}
	err = db.Get(t, "SELECT * FROM tb_task WHERE id=?", id)

	return t, err
}

func ListTask() ([]Task, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	list := make([]Task, 0, 50)
	err = db.Select(&list, "SELECT * FROM tb_task")

	return list, err
}

func ListTaskByStatus(status int) ([]Task, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	list := make([]Task, 0, 50)
	err = db.Select(&list, "SELECT * FROM tb_task WHERE status=?", status)

	return list, err
}

func ListTaskByRelated(related string) ([]Task, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	list := make([]Task, 0, 50)
	err = db.Select(&list, "SELECT * FROM tb_task WHERE related=?", related)

	return list, err
}

func DeleteTask(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_task WHERE id=?", id)

	return err
}

const insertBackupStrategyQuery = "INSERT INTO tb_backup_strategy (id,name,type,service_id,spec,next,valid,enabled,backup_dir,timeout,created_at) VALUES (:id,:name,:type,:service_id,:spec,:next,:valid,:enabled,:backup_dir,:timeout,:created_at)"

type BackupStrategy struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	ServiceID string    `db:"service_id"`
	Type      string    `db:"type"` // full/part
	Spec      string    `db:"spec"`
	BackupDir string    `db:"backup_dir"`
	Next      time.Time `db:"next"`
	Valid     time.Time `db:"valid"`
	Enabled   bool      `db:"enabled"`
	Timeout   int       `db:"timeout"` // s
	CreatedAt time.Time `db:"created_at"`
}

func (bs BackupStrategy) TableName() string {
	return "tb_backup_strategy"
}

func GetBackupStrategy(nameOrID string) (*BackupStrategy, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	strategy := &BackupStrategy{}
	query := "SELECT * FROM tb_backup_strategy WHERE id=? OR name=?"

	err = db.Get(strategy, query, nameOrID, nameOrID)
	if err != nil {
		return nil, err
	}

	return strategy, nil
}

func ListBackupStrategyByServiceID(id string) ([]BackupStrategy, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	strategy := make([]BackupStrategy, 0, 3)
	query := "SELECT * FROM tb_backup_strategy WHERE service_id=?"

	err = db.Select(&strategy, query, id)
	if err != nil {
		return nil, err
	}

	return strategy, nil
}

func UpdateBackupStrategyStatus(id string, enable bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_backup_strategy SET enabled=? WHERE id=?", enable, id)

	return err
}

func (bs *BackupStrategy) UpdateNext(next time.Time, enable bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_backup_strategy SET next=?,enabled=? WHERE id=?"
	_, err = db.Exec(query, next, enable, bs.ID)
	if err == nil {
		bs.Next = next
		bs.Enabled = enable
	}

	return err
}

func DeleteBackupStrategy(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return fmt.Errorf("DB Error:%s", err)
	}

	_, err = db.Exec("DELETE FROM tb_backup_strategy WHERE id=?", id)
	if err != nil {
		return err
	}

	return nil
}

func txDeleteBackupStrategy(tx *sqlx.Tx, id string) error {
	_, err := tx.Exec("DELETE FROM tb_backup_strategy WHERE id=? OR service_id=?", id, id)

	return err

}

func TxInsertBackupStrategy(tx *sqlx.Tx, strategy BackupStrategy) error {
	_, err := tx.NamedExec(insertBackupStrategyQuery, &strategy)

	return err
}

func TxInsertBackupStrategyAndTask(strategy BackupStrategy, task Task) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = TxInsertBackupStrategy(tx, strategy)
	if err != nil {
		return err
	}

	err = TxInsertTask(tx, task)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func InsertBackupStrategy(strategy BackupStrategy) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}
	_, err = db.NamedExec(insertBackupStrategyQuery, &strategy)

	return err
}

func UpdateBackupStrategy(strategy BackupStrategy) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}
	query := "UPDATE tb_backup_strategy SET name=:name,type=:type,service_id=:service_id,spec=:spec,next=:next,valid=:valid,enabled=:enabled,backup_dir=:backup_dir,timeout=:timeout,created_at=:created_at WHERE id=:id"
	_, err = db.NamedExec(query, &strategy)

	return err
}

func BackupTaskValidate(taskID, strategyID, unitID string) (Task, int, error) {
	db, err := GetDB(true)
	if err != nil {
		return Task{}, 0, err
	}

	task := Task{}
	err = db.Get(&task, "SELECT * FROM tb_task WHERE id=?", taskID)
	if err != nil {
		return task, 0, err
	}

	service := ""
	err = db.Get(&service, "SELECT service_id FROM tb_backup_strategy WHERE id=?", strategyID)
	if err != nil {
		return task, 0, err
	}

	unit := Unit{}
	err = db.Get(&unit, "SELECT * FROM tb_unit WHERE id=?", unitID)

	rent := 0
	err = db.Get(&rent, "SELECT backup_files_retention FROM tb_service WHERE id=?", service)

	return task, rent, err
}
