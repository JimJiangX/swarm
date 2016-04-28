package database

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
)

type Task struct {
	ID string `db:"id"`
	//	Name        string        `db:"name"`
	Related     string        `db:"related"`
	Linkto      string        `db:"link_to"`
	Description string        `db:"description"`
	Labels      string        `db:"labels"`
	Errors      string        `db:"errors"`
	Timeout     time.Duration `db:"timeout"` // s
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
	SizeByte   int       `db:"size"`
	Retention  time.Time `db:"retention"`
	CreatedAt  time.Time `db:"created_at"`
}

func (bf BackupFile) TableName() string {
	return "tb_backup_file"
}

func txInsertBackupFile(tx *sqlx.Tx, bf BackupFile) error {
	query := "INSERT INTO tb_backup_file (id,task_id,strategy_id,unit_id,type,path,size,retention,created_at) VALUES (:id,:task_id,:strategy_id,:unit_id,:type,:path,:size,:retention,:created_at)"
	_, err := tx.NamedExec(query, &bf)

	return err
}

func NewTask(relate, linkto, des string, labels []string, timeout time.Duration) *Task {
	return &Task{
		ID: utils.Generate64UUID(),
		//	Name:        name,
		Related:     relate,
		Linkto:      linkto,
		Description: des,
		Labels:      strings.Join(labels, "&;&"),
		Timeout:     timeout,
		Status:      0,
		CreatedAt:   time.Now(),
	}
}

func InsertTask(task *Task) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}
	query := "INSERT INTO tb_task (id,related,link_to,description,labels,errors,timeout,status,create_at,finished_at) VALUES (:id,:related,:link_to,:description,:labels,:errors,:timeout,:status,:create_at,:finished_at)"

	_, err = db.NamedExec(query, task)

	return err
}

func TxInsertTask(tx *sqlx.Tx, t *Task) error {
	query := "INSERT INTO tb_task (id,related,link_to,description,labels,errors,timeout,status,create_at,finished_at) VALUES (:id,:related,:link_to,:description,:labels,:errors,:timeout,:status,:create_at,:finished_at)"

	_, err := tx.NamedExec(query, t)

	return err
}

func TxInsertMultiTask(tx *sqlx.Tx, tasks []*Task) error {
	query := "INSERT INTO tb_task (id,related,link_to,description,labels,errors,timeout,status,create_at,finished_at) VALUES (:id,:related,:link_to,:description,:labels,:errors,:timeout,:status,:create_at,:finished_at)"

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

		atomic.StoreInt32(&task.Status, int32(state))

		return nil
	}

	_, err = db.Exec(query, state, finishAt, msg, task.ID)
	if err != nil {
		return err
	}

	atomic.StoreInt32(&task.Status, int32(state))
	task.FinishedAt = finishAt

	return nil
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

func DeleteTask(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_task WHERE id=?", id)

	return err
}

type BackupStrategy struct {
	ID          string        `db:"id"`
	Type        string        `db:"type"` // full/part
	Spec        string        `db:"spec"`
	Next        time.Time     `db:"next"`
	Valid       time.Time     `db:"valid"`
	Enabled     bool          `db:"enabled"`
	BackupDir   string        `db:"backup_dir"`
	MaxSizeByte int           `db:"max_size"`
	Retention   time.Duration `db:"retention"`
	Timeout     time.Duration `db:"timeout"` // s
	CreatedAt   time.Time     `db:"create_at"`
}

func (bs BackupStrategy) TableName() string {
	return "tb_backup_strategy"
}

func GetBackupStrategy(id string) (*BackupStrategy, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	strategy := &BackupStrategy{}
	query := "SELECT * FROM tb_backup_strategy WHERE id=?"

	err = db.QueryRowx(query, id).StructScan(strategy)
	if err != nil {
		return nil, err
	}

	return strategy, nil
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

func TxInsertBackupStrategy(tx *sqlx.Tx, strategy *BackupStrategy) error {
	query := "INSERT INTO tb_backup_strategy (id,type,spec,next,valid,enabled,backup_dir,max_size,retention,timeout,create_at) VALUES (:id,:type,:spec,:next,:valid,:enabled,:backup_dir,:max_size,:retention,:timeout,:create_at)"
	_, err := tx.NamedExec(query, strategy)

	return err
}

func BackupTaskValidate(taskID, strategyID, unitID string) (int, error) {
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	state := 0
	err = db.Get(&state, "SELECT status FROM tb_task WHERE id=?", taskID)
	if err != nil {
		return 0, err
	}

	rent := 0
	err = db.QueryRowx("SELECT retention FROM tb_backup_strategy WHERE id=?", strategyID).Scan(&rent)
	if err != nil {
		return 0, err
	}

	err = db.Get(&state, "SELECT status FROM tb_unit WHERE id=?", unitID)

	return rent, err

}
