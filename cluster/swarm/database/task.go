package database

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertTaskQuery = "INSERT INTO tb_task (id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

type Task struct {
	ID          string    `db:"id"`
	Name        string    `db:"name"` //Related-Object
	Related     string    `db:"related"`
	Linkto      string    `db:"link_to"`
	Description string    `db:"description"`
	Labels      string    `db:"labels"`
	Errors      string    `db:"errors"`
	Status      int64     `db:"status"`
	Timeout     int       `db:"timeout"`   // s
	Timestamp   int64     `db:"timestamp"` // time.Time.Unix()
	CreatedAt   time.Time `db:"created_at"`
	FinishedAt  time.Time `db:"finished_at"`
}

func (t Task) tableName() string {
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

func (bf BackupFile) tableName() string {
	return "tb_backup_files"
}

func ListBackupFilesByService(nameOrID string) (Service, []BackupFile, error) {
	db, err := GetDB(true)
	if err != nil {
		return Service{}, nil, err
	}

	service := Service{}
	err = db.Get(&service, "SELECT * FROM tb_service WHERE id=? OR name=?", nameOrID, nameOrID)
	if err != nil {
		return service, nil, errors.Wrapf(err, "Not Found Service By '%s'", nameOrID)
	}

	var units []string
	err = db.Select(&units, "SELECT id FROM tb_unit WHERE service_id=?", service.ID)
	if err != nil {
		return service, nil, errors.Wrapf(err, "Not Found Units By service_id='%s'", service.ID)
	}

	if len(units) == 0 {
		return service, []BackupFile{}, nil
	}

	query, args, err := sqlx.In("SELECT * FROM tb_backup_files WHERE unit_id IN (?);", units)
	if err != nil {
		return service, nil, err
	}

	var files []BackupFile
	err = db.Select(&files, query, args...)
	if err != nil {
		return service, nil, errors.Wrapf(err, "Not Found BackupFile By unit_id='%s'", units)
	}

	return service, files, nil
}

func GetBackupFile(id string) (BackupFile, error) {
	db, err := GetDB(false)
	if err != nil {
		return BackupFile{}, err
	}

	row := BackupFile{}
	const query = "SELECT * FROM tb_backup_files WHERE id=?"

	err = db.Get(&row, query, id)
	if err == nil {
		return row, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return BackupFile{}, err
	}

	err = db.Get(&row, query, id)
	if err == nil {
		return row, nil
	}

	return row, errors.Wrap(err, "Get Backup File By ID:"+id)
}

func txInsertBackupFile(tx *sqlx.Tx, bf BackupFile) error {
	_, err := tx.NamedExec(insertBackupFileQuery, &bf)

	return err
}

func NewTask(object, relate, linkto, des string, labels []string, timeout int) Task {
	return Task{
		ID:          utils.Generate64UUID(),
		Name:        relate + "-" + object,
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
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	task.Timestamp = task.CreatedAt.Unix()

	_, err = db.NamedExec(insertTaskQuery, &task)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertTaskQuery, &task)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Insert Task")
}

func TxInsertTask(tx *sqlx.Tx, t Task) error {

	t.Timestamp = t.CreatedAt.Unix()

	_, err := tx.NamedExec(insertTaskQuery, &t)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx Insert Task")
}

func TxInsertMultiTask(tx *sqlx.Tx, tasks []*Task) error {
	stmt, err := tx.PrepareNamed(insertTaskQuery)
	if err != nil {
		return errors.Wrap(err, "tx prepare insert task")
	}

	for i := range tasks {
		if tasks[i] == nil {
			continue
		}

		tasks[i].Timestamp = tasks[i].CreatedAt.Unix()

		_, err = stmt.Exec(tasks[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx insert []Task")
		}
	}

	err = stmt.Close()

	return errors.Wrap(err, "Tx insert []Task")
}

func txUpdateTaskStatus(tx *sqlx.Tx, t *Task, state int64, finish time.Time, msg string) error {
	query := "UPDATE tb_task SET status=?,finished_at=?,errors=? WHERE id=?"

	if finish.IsZero() {
		query = "UPDATE tb_task SET status=?,errors=? WHERE id=?"

		_, err := tx.Exec(query, state, msg, t.ID)
		if err != nil {
			return errors.Wrap(err, "Tx update Task status")
		}

		atomic.StoreInt64(&t.Status, state)

		return nil
	}

	_, err := tx.Exec(query, state, finish, msg, t.ID)
	if err != nil {
		return errors.Wrap(err, "Tx update Task status")
	}

	atomic.StoreInt64(&t.Status, state)
	t.FinishedAt = finish

	return nil
}

func TxBackupTaskDone(task *Task, state int64, backupFile BackupFile) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txInsertBackupFile(tx, backupFile)
	if err != nil {
		return err
	}

	err = txUpdateTaskStatus(tx, task, state, time.Now(), "")
	if err != nil {
		return err
	}

	return tx.Commit()
}

func UpdateTaskStatus(task *Task, state int64, finishAt time.Time, msg string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_task SET status=?,finished_at=?,errors=? WHERE id=?"

	if finishAt.IsZero() {
		query = "UPDATE tb_task SET status=?,errors=? WHERE id=?"

		_, err := db.Exec(query, state, msg, task.ID)
		if err != nil {
			return errors.Wrap(err, "Update Task Status")
		}

		atomic.StoreInt64(&task.Status, state)

		return nil
	}

	_, err = db.Exec(query, state, finishAt, msg, task.ID)
	if err != nil {
		return errors.Wrap(err, "Update Task Status")
	}

	atomic.StoreInt64(&task.Status, state)
	task.FinishedAt = finishAt

	return nil
}

func (task *Task) UpdateStatus(status int, msg string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	now := time.Now()
	const query = "UPDATE tb_task SET status=?,finished_at=?,errors=? WHERE id=?"

	_, err = db.Exec(query, status, now, msg, task.ID)
	if err == nil {
		atomic.StoreInt64(&task.Status, int64(status))
		task.FinishedAt = now

		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, status, now, msg, task.ID)
	if err == nil {
		atomic.StoreInt64(&task.Status, int64(status))
		task.FinishedAt = now

		return nil
	}

	return errors.Wrap(err, "Update Task Status")
}

func GetTask(id string) (*Task, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	task := &Task{}
	const query = "SELECT * FROM tb_task WHERE id=?"

	err = db.Get(task, query, id)
	if err == nil {
		return task, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Get(task, query, id)
	if err == nil {
		return task, nil
	}

	return nil, errors.Wrap(err, "Get Task By ID:"+id)
}

func ListTask() ([]Task, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []Task
	const query = "SELECT * FROM tb_task"

	err = db.Select(&list, query)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "Select []Task")
}

func ListTaskByStatus(status int) ([]Task, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []Task
	const query = "SELECT * FROM tb_task WHERE status=?"

	err = db.Select(&list, query, status)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, status)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrapf(err, "Select []Task By Status:%d", status)
}

func ListTaskByRelated(related string) ([]Task, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []Task
	const query = "SELECT * FROM tb_task WHERE related=?"

	err = db.Select(&list, query, related)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, related)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "Select []Task By Related:"+related)
}

func ListTaskByTimestamp(begin, end time.Time) ([]Task, error) {
	if begin.After(end) {
		begin, end = end, begin
	}

	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var (
		list  []Task
		min   = begin.Unix()
		max   = end.Unix()
		query = "SELECT * FROM tb_task"
	)

	if begin.IsZero() && !end.IsZero() {

		query = "SELECT * FROM tb_task WHERE timestamp<=?" //max
		err = db.Select(&list, query, max)

	} else if !begin.IsZero() && end.IsZero() {

		query = "SELECT * FROM tb_task WHERE timestamp>=?" //min
		err = db.Select(&list, query, min)

	} else if !begin.IsZero() && !end.IsZero() {

		query = "SELECT * FROM tb_task WHERE timestamp>=? AND timestamp<=?" //max
		err = db.Select(&list, query, min, max)

	} else {
		err = db.Select(&list, query)
	}

	if err != nil {
		err = errors.Wrapf(err, "Select Task By Timestamp,begin=%s,end=%s", begin, end)
	}

	return list, err
}

func DeleteTask(id string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tb_task WHERE id=?"

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete Task By ID:"+id)
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

func (bs BackupStrategy) tableName() string {
	return "tb_backup_strategy"
}

func GetBackupStrategy(nameOrID string) (*BackupStrategy, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	strategy := &BackupStrategy{}
	const query = "SELECT * FROM tb_backup_strategy WHERE id=? OR name=?"

	err = db.Get(strategy, query, nameOrID, nameOrID)
	if err == nil {
		return strategy, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Get(strategy, query, nameOrID, nameOrID)
	if err == nil {
		return strategy, nil
	}

	return nil, errors.Wrap(err, "Get BackupStrategy By nameOrID:"+nameOrID)
}

func ListBackupStrategyByServiceID(id string) ([]BackupStrategy, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var strategies []BackupStrategy
	const query = "SELECT * FROM tb_backup_strategy WHERE service_id=?"

	err = db.Select(&strategies, query, id)
	if err == nil {
		return strategies, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&strategies, query, id)
	if err == nil {
		return strategies, nil
	}

	return nil, errors.Wrap(err, "Select []BackupStrategy By ServiceID:"+id)
}

func UpdateBackupStrategyStatus(id string, enable bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_backup_strategy SET enabled=? WHERE id=?"

	_, err = db.Exec(query, enable, id)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, enable, id)
	if err == nil {
		return nil
	}

	return errors.Wrapf(err, "Update BackupStrategy By ID=%s,Enabled=%t", id, enable)
}

func (bs *BackupStrategy) UpdateNext(next time.Time, enable bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_backup_strategy SET next=?,enabled=? WHERE id=?"

	_, err = db.Exec(query, next, enable, bs.ID)
	if err == nil {
		bs.Next = next
		bs.Enabled = enable
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, next, enable, bs.ID)
	if err == nil {
		bs.Next = next
		bs.Enabled = enable
	}

	return errors.Wrap(err, "Update BackupStrategy Next By ID:"+bs.ID)
}

func DeleteBackupStrategy(id string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tb_backup_strategy WHERE id=?"

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete BackupStrategy By ID:"+id)
}

func txDeleteBackupStrategy(tx *sqlx.Tx, id string) error {
	_, err := tx.Exec("DELETE FROM tb_backup_strategy WHERE id=? OR service_id=?", id, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx Delete BackupStrategy By ID:"+id)
}

func TxInsertBackupStrategy(tx *sqlx.Tx, strategy BackupStrategy) error {
	_, err := tx.NamedExec(insertBackupStrategyQuery, &strategy)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx Insert BackupStrategy")
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
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertBackupStrategyQuery, &strategy)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertBackupStrategyQuery, &strategy)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Insert BackupStrategy")
}

func UpdateBackupStrategy(strategy BackupStrategy) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_backup_strategy SET name=:name,type=:type,service_id=:service_id,spec=:spec,next=:next,valid=:valid,enabled=:enabled,backup_dir=:backup_dir,timeout=:timeout,created_at=:created_at WHERE id=:id"

	_, err = db.NamedExec(query, &strategy)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(query, &strategy)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Update BackupStrategy")
}

func BackupTaskValidate(taskID, strategyID, unitID string) (Task, int, error) {
	db, err := GetDB(true)
	if err != nil {
		return Task{}, 0, err
	}

	task := Task{}
	err = db.Get(&task, "SELECT * FROM tb_task WHERE id=?", taskID)

	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: Get Task By ID:"+taskID)
	}

	service := ""
	err = db.Get(&service, "SELECT service_id FROM tb_backup_strategy WHERE id=?", strategyID)
	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: Get BackupStrategy By ID:"+strategyID)
	}

	unit := Unit{}
	err = db.Get(&unit, "SELECT * FROM tb_unit WHERE id=?", unitID)
	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: Get Unit By ID:"+unitID)
	}

	rent := 0
	err = db.Get(&rent, "SELECT backup_files_retention FROM tb_service WHERE id=?", service)
	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: Get Service By ID:"+service)
	}

	return task, rent, nil
}
