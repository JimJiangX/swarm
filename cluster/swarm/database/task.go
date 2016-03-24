package database

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
)

type Task struct {
	ID          string        `db:"id"`
	Name        string        `db:"name"`
	Related     string        `db:"related"`
	Linkto      string        `db:"link_to"`
	Description string        `db:"description"`
	Labels      string        `db:"labels"`
	Errors      string        `db:"errors"`
	Timeout     time.Duration `db:"timeout"`
	Status      int32         `db:"status"`
	CreatedAt   time.Time     `db:"create_at"`
	FinishedAt  time.Time     `db:"finished_at"`
}

func (t Task) TableName() string {
	return "tb_task"
}

type BackupFile struct {
	ID         string    `db:"id"`
	TaskID     string    `db:"task_id"`
	StrategyID string    `db:"strategy_id"`
	UnitID     string    `db:"unit_id"`
	Type       string    `db:"type"` // full or incremental
	Path       string    `db:"path"`
	SizeByte   int64     `db:"size"`
	Status     byte      `db:"status"`
	CreatedAt  time.Time `db:"created_at"`
}

func (bf BackupFile) TableName() string {
	return "tb_backup_file"
}

type BackupStrategy struct {
	ID          string        `db:"id"`
	Type        string        `db:"type"` // full/part
	Spec        string        `db:"spec"`
	Next        time.Time     `db:"next"`
	Valid       time.Time     `db:"valid"`
	Enabled     bool          `db:"enabled"`
	BackupDir   string        `db:"backup_dir"`
	MaxSizeByte int64         `db:"max_size"`
	Retention   time.Duration `db:"retention"`
	Timeout     time.Duration `db:"timeout"`
	Status      byte          `db:"status"`
	CreatedAt   time.Time     `db:"create_at"`
}

func (bs BackupStrategy) TableName() string {
	return "tb_backup_strategy"
}

func NewTask(name, relate, linkto, des string, labels []string, timeout time.Duration) *Task {
	return &Task{
		ID:          "",
		Name:        name,
		Related:     relate,
		Linkto:      linkto,
		Description: des,
		Labels:      strings.Join(labels, "&;&"),
		Timeout:     timeout,
		CreatedAt:   time.Now(),
	}
}

func TxInsertTask(tx *sqlx.Tx, t *Task) error {
	query := "INSERT INTO tb_task (id,name,related,link_to,description,labels,errors,timeout,status,create_at,finished_at) VALUES (:id,:name,:related,:link_to,:description,:labels,:errors,:timeout,:status,:create_at,:finished_at)"

	_, err := tx.NamedExec(query, t)

	return err
}

func TxInsertMultiTask(tx *sqlx.Tx, tasks []*Task) error {
	query := "INSERT INTO tb_task (id,name,related,link_to,description,labels,errors,timeout,status,create_at,finished_at) VALUES (:id,:name,:related,:link_to,:description,:labels,:errors,:timeout,:status,:create_at,:finished_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}

	for i := range tasks {
		if tasks[i] == nil {
			continue
		}

		_, err = stmt.Exec(tasks[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func TxUpdateTaskStatus(tx *sqlx.Tx, t *Task, state int, finish time.Time) error {
	query := "UPDATE tb_task SET status=?,finished_at=? WHERE id=?"

	if finish.IsZero() {
		query = "UPDATE tb_task SET status=? WHERE id=?"

		_, err := tx.Exec(query, state, t.ID)
		if err == nil {
			atomic.StoreInt32(&t.Status, int32(state))
		}

		return err
	}

	_, err := tx.Exec(query, state, finish, t.ID)
	if err == nil {

		atomic.StoreInt32(&t.Status, int32(state))
		t.FinishedAt = finish
	}

	return err
}

func QueryTask(id string) (*Task, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	t := &Task{}
	err = db.QueryRowx("SELECT * FROM tb_task WHERE id=?", id).StructScan(t)

	return t, err
}
