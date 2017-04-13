package database

import (
	"database/sql"
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/swarm/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertTaskQuery = "INSERT INTO tbl_dbaas_task (id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

// Task is table tbl_dbaas_task structure,record tasks status
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
	return "tbl_dbaas_task"
}

const insertBackupFileQuery = "INSERT INTO tbl_dbaas_backup_files (id,task_id,strategy_id,unit_id,type,path,size,retention,created_at,finished_at) VALUES (:id,:task_id,:strategy_id,:unit_id,:type,:path,:size,:retention,:created_at,:finished_at)"

// BackupFile is table tbl_dbaas_backup_files structure,correspod with backup files
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
	return "tbl_dbaas_backup_files"
}

// ListBackupFiles return all BackupFile
func ListBackupFiles() ([]BackupFile, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var out []BackupFile

	err = db.Select(&out, "SELECT id,task_id,strategy_id,unit_id,type,path,size,retention,created_at,finished_at FROM tbl_dbaas_backup_files")
	if err == nil {
		return out, nil
	} else if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, errors.Wrap(err, "list []BackupFile")
}

// ListBackupFilesByService returns Service and []BackupFile select by name or ID
func ListBackupFilesByService(nameOrID string) (Service, []BackupFile, error) {
	db, err := getDB(true)
	if err != nil {
		return Service{}, nil, err
	}

	defer func() {
		if errors.Cause(err) == sql.ErrNoRows {
			err = nil
		}
	}()

	service := Service{}
	err = db.Get(&service, "SELECT id,name,description,architecture,business_code,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at FROM tbl_dbaas_service WHERE id=? OR name=?", nameOrID, nameOrID)
	if err != nil {
		return service, nil, errors.Wrapf(err, "not found Service by '%s'", nameOrID)
	}

	var units []string
	err = db.Select(&units, "SELECT id FROM tbl_dbaas_unit WHERE service_id=?", service.ID)
	if err != nil {
		return service, nil, errors.Wrapf(err, "not found Units by service_id='%s'", service.ID)
	}

	if len(units) == 0 {
		return service, []BackupFile{}, nil
	}

	query, args, err := sqlx.In("SELECT id,task_id,strategy_id,unit_id,type,path,size,retention,created_at,finished_at FROM tbl_dbaas_backup_files WHERE unit_id IN (?);", units)
	if err != nil {
		return service, nil, errors.Wrap(err, "select BackupFile by UnitIDs")
	}

	var files []BackupFile
	err = db.Select(&files, query, args...)
	if err != nil {
		return service, nil, errors.Wrapf(err, "not found BackupFile by unit_id='%s'", units)
	}

	return service, files, nil
}

// GetBackupFile returns BackupFile select by ID
func GetBackupFile(id string) (BackupFile, error) {
	db, err := getDB(false)
	if err != nil {
		return BackupFile{}, err
	}

	row := BackupFile{}
	const query = "SELECT id,task_id,strategy_id,unit_id,type,path,size,retention,created_at,finished_at FROM tbl_dbaas_backup_files WHERE id=?"

	err = db.Get(&row, query, id)
	if err == nil {
		return row, nil
	}

	db, err = getDB(true)
	if err != nil {
		return BackupFile{}, err
	}

	err = db.Get(&row, query, id)

	return row, errors.Wrap(err, "get Backup File by ID")
}

func txInsertBackupFile(tx *sqlx.Tx, bf BackupFile) error {
	_, err := tx.NamedExec(insertBackupFileQuery, &bf)

	return errors.Wrap(err, "Tx insert BackupFile")
}

func DelBackupFile(ID string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tbl_dbaas_backup_files WHERE id=?", ID)

	return errors.Wrap(err, "del backup file")
}

// NewTask new a Task
func NewTask(object, relate, linkto, des string, labels []string, timeout int) Task {
	return Task{
		ID:          utils.Generate64UUID(),
		Name:        relate + "-" + object,
		Related:     relate,
		Linkto:      linkto,
		Description: des,
		Labels:      strings.Join(labels, ";"),
		Timeout:     timeout,
		Status:      0,
		CreatedAt:   time.Now(),
	}
}

// Insert insert a Task
func (t Task) Insert() error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	t.Timestamp = t.CreatedAt.Unix()

	_, err = db.NamedExec(insertTaskQuery, &t)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertTaskQuery, &t)

	return errors.Wrap(err, "insert Task")
}

// TxInsertTask insert a Task in Tx
func TxInsertTask(tx *sqlx.Tx, t Task) error {
	t.Timestamp = t.CreatedAt.Unix()

	_, err := tx.NamedExec(insertTaskQuery, &t)

	return errors.Wrap(err, "Tx insert Task")
}

// TxInsertMultiTask insert []*Task in Tx
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
	query := "UPDATE tbl_dbaas_task SET status=?,finished_at=?,errors=? WHERE id=?"

	if finish.IsZero() {
		query = "UPDATE tbl_dbaas_task SET status=?,errors=? WHERE id=?"

		_, err := tx.Exec(query, state, msg, t.ID)
		if err != nil {
			return errors.Wrap(err, "Tx update Task status")
		}

		atomic.StoreInt64(&t.Status, state)

		return nil
	}

	_, err := tx.Exec(query, state, finish, msg, t.ID)
	if err != nil {
		return errors.Wrap(err, "Tx update Task status & errors")
	}

	atomic.StoreInt64(&t.Status, state)
	t.FinishedAt = finish

	return nil
}

// TxBackupTaskDone insert BackupFile and update Task status in Tx
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

	err = tx.Commit()

	return errors.Wrap(err, "Tx insert BackupFile and update Task status")
}

// UpdateTaskStatus update Task status
func UpdateTaskStatus(task *Task, state int64, finishAt time.Time, msg string) error {
	db, err := getDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tbl_dbaas_task SET status=?,finished_at=?,errors=? WHERE id=?"

	if finishAt.IsZero() {
		query = "UPDATE tbl_dbaas_task SET status=?,errors=? WHERE id=?"

		_, err := db.Exec(query, state, msg, task.ID)
		if err != nil {
			return errors.Wrap(err, "update Task status")
		}

		atomic.StoreInt64(&task.Status, state)

		return nil
	}

	_, err = db.Exec(query, state, finishAt, msg, task.ID)
	if err != nil {
		return errors.Wrap(err, "update Task status")
	}

	atomic.StoreInt64(&task.Status, state)
	task.FinishedAt = finishAt

	return nil
}

// UpdateStatus update Task status
func (t *Task) UpdateStatus(status int, msg string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	now := time.Now()
	const query = "UPDATE tbl_dbaas_task SET status=?,finished_at=?,errors=? WHERE id=?"

	_, err = db.Exec(query, status, now, msg, t.ID)
	if err == nil {
		atomic.StoreInt64(&t.Status, int64(status))
		t.FinishedAt = now

		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, status, now, msg, t.ID)
	if err == nil {
		atomic.StoreInt64(&t.Status, int64(status))
		t.FinishedAt = now

		return nil
	}

	return errors.Wrap(err, "update Task status")
}

// GetTask returns Task select by ID
func GetTask(id string) (Task, error) {
	db, err := getDB(false)
	if err != nil {
		return Task{}, err
	}

	task := Task{}
	const query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task WHERE id=?"

	err = db.Get(&task, query, id)
	if err == nil {
		return task, nil
	}

	db, err = getDB(true)
	if err != nil {
		return task, err
	}

	err = db.Get(&task, query, id)

	return task, errors.Wrap(err, "get Task by ID")
}

// ListTask returns all []Task
func ListTask() ([]Task, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var list []Task
	const query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task"

	err = db.Select(&list, query)
	if err == nil {
		return list, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query)

	return list, errors.Wrap(err, "list all []Task")
}

// ListTaskByStatus returns []Task select by status
func ListTaskByStatus(status int) ([]Task, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var list []Task
	const query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task WHERE status=?"

	err = db.Select(&list, query, status)
	if err == nil {
		return list, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, status)

	return list, errors.Wrapf(err, "list []Task By Status")
}

// ListTaskByRelated returns []Task select by Related
func ListTaskByRelated(related string) ([]Task, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var list []Task
	const query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task WHERE related=?"

	err = db.Select(&list, query, related)
	if err == nil {
		return list, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, related)

	return list, errors.Wrap(err, "list []Task by Related")
}

// ListTaskByTimestamp returns []Task by Timestamp
// begin is default 0,end default unlimit
func ListTaskByTimestamp(begin, end time.Time) ([]Task, error) {
	if begin.After(end) {
		begin, end = end, begin
	}

	db, err := getDB(true)
	if err != nil {
		return nil, err
	}

	var (
		list  []Task
		min   = begin.Unix()
		max   = end.Unix()
		query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task"
	)

	if begin.IsZero() && !end.IsZero() {

		query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task WHERE timestamp<=?" //max
		err = db.Select(&list, query, max)

	} else if !begin.IsZero() && end.IsZero() {

		query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task WHERE timestamp>=?" //min
		err = db.Select(&list, query, min)

	} else if !begin.IsZero() && !end.IsZero() {

		query = "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task WHERE timestamp>=? AND timestamp<=?" //max
		err = db.Select(&list, query, min, max)

	} else {
		err = db.Select(&list, query)
	}

	if err != nil {
		err = errors.Wrapf(err, "list []Task by Timestamp,begin=%s,end=%s", begin, end)
	}

	return list, err
}

// DeleteTask delete a Task by ID
func DeleteTask(ID string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tbl_dbaas_task WHERE id=?"

	_, err = db.Exec(query, ID)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, ID)

	return errors.Wrap(err, "delete Task by ID")
}

const insertBackupStrategyQuery = "INSERT INTO tbl_dbaas_backup_strategy (id,name,type,service_id,spec,next,valid,enabled,backup_dir,timeout,created_at) VALUES (:id,:name,:type,:service_id,:spec,:next,:valid,:enabled,:backup_dir,:timeout,:created_at)"

// BackupStrategy is table tbl_dbaas_backup_Strategy,
// correspod with Service BackupStrategy
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
	return "tbl_dbaas_backup_strategy"
}

// GetBackupStrategy returns *BackupStrategy select by name or ID
func GetBackupStrategy(nameOrID string) (*BackupStrategy, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var strategy BackupStrategy
	const query = "SELECT id,name,type,service_id,spec,next,valid,enabled,backup_dir,timeout,created_at FROM tbl_dbaas_backup_strategy WHERE id=? OR name=?"

	err = db.Get(&strategy, query, nameOrID, nameOrID)
	if err == nil {
		return &strategy, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Get(&strategy, query, nameOrID, nameOrID)
	if err == nil {
		return &strategy, nil
	}

	return nil, errors.Wrap(err, "get BackupStrategy by nameOrID:"+nameOrID)
}

// ListBackupStrategyByServiceID returns []BackupStrategy select by serviceID
func ListBackupStrategyByServiceID(serviceID string) ([]BackupStrategy, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var strategies []BackupStrategy
	const query = "SELECT id,name,type,service_id,spec,next,valid,enabled,backup_dir,timeout,created_at FROM tbl_dbaas_backup_strategy WHERE service_id=?"

	err = db.Select(&strategies, query, serviceID)
	if err == nil {
		return strategies, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&strategies, query, serviceID)

	return strategies, errors.Wrap(err, "list []BackupStrategy by ServiceID")
}

// UpdateBackupStrategyStatus update BackupStrategy Enabled
func UpdateBackupStrategyStatus(id string, enable bool) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_backup_strategy SET enabled=? WHERE id=?"

	_, err = db.Exec(query, enable, id)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, enable, id)

	return errors.Wrap(err, "update BackupStrategy by ID")
}

// UpdateNext update BackupStrategy Next\Enabled
func (bs *BackupStrategy) UpdateNext(next time.Time, enable bool) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_backup_strategy SET next=?,enabled=? WHERE id=?"

	_, err = db.Exec(query, next, enable, bs.ID)
	if err == nil {
		bs.Next = next
		bs.Enabled = enable

		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, next, enable, bs.ID)
	if err == nil {
		bs.Next = next
		bs.Enabled = enable

		return nil
	}

	return errors.Wrap(err, "update BackupStrategy Next by ID")
}

// DeleteBackupStrategy delete BackupStrategy by ID
func DeleteBackupStrategy(ID string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tbl_dbaas_backup_strategy WHERE id=?"

	_, err = db.Exec(query, ID)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, ID)

	return errors.Wrap(err, "delete BackupStrategy by ID")
}

func txDeleteBackupStrategy(tx *sqlx.Tx, id string) error {
	_, err := tx.Exec("DELETE FROM tbl_dbaas_backup_strategy WHERE id=? OR service_id=?", id, id)

	return errors.Wrap(err, "Tx delete BackupStrategy by ID")
}

// TxInsertBackupStrategy insert BackupStrategy in Tx
func TxInsertBackupStrategy(tx *sqlx.Tx, strategy BackupStrategy) error {
	_, err := tx.NamedExec(insertBackupStrategyQuery, &strategy)

	return errors.Wrap(err, "Tx insert BackupStrategy")
}

// TxInsertBackupStrategyAndTask insert BackupStrategy and Task in a Tx
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

	err = tx.Commit()

	return errors.Wrap(err, "Tx insert BackupStrategy and Task")
}

// InsertBackupStrategy insert BackupStrategy
func InsertBackupStrategy(strategy BackupStrategy) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertBackupStrategyQuery, &strategy)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertBackupStrategyQuery, &strategy)

	return errors.Wrap(err, "insert BackupStrategy")
}

// UpdateBackupStrategy update BackupStrategy
func UpdateBackupStrategy(strategy BackupStrategy) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_backup_strategy SET name=:name,type=:type,service_id=:service_id,spec=:spec,next=:next,valid=:valid,enabled=:enabled,backup_dir=:backup_dir,timeout=:timeout,created_at=:created_at WHERE id=:id"

	_, err = db.NamedExec(query, &strategy)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(query, &strategy)

	return errors.Wrap(err, "update BackupStrategy")
}

// BackupTaskValidate valid taskID\strategyID\unitID if all exist,
// if true,returns the Task
func BackupTaskValidate(taskID, strategyID, unitID string) (Task, int, error) {
	db, err := getDB(true)
	if err != nil {
		return Task{}, 0, err
	}

	task := Task{}
	err = db.Get(&task, "SELECT id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM tbl_dbaas_task WHERE id=?", taskID)

	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: get Task by ID:"+taskID)
	}

	service := ""
	err = db.Get(&service, "SELECT service_id FROM tbl_dbaas_backup_strategy WHERE id=?", strategyID)
	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: get BackupStrategy by ID:"+strategyID)
	}

	unit := Unit{}
	err = db.Get(&unit, "SELECT id,name,type,image_id,image_name,service_id,node_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at FROM tbl_dbaas_unit WHERE id=?", unitID)
	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: get Unit by ID:"+unitID)
	}

	rent := 0
	err = db.Get(&rent, "SELECT backup_files_retention FROM tbl_dbaas_service WHERE id=?", service)
	if err != nil {
		return task, 0, errors.Wrap(err, "BackupTaskValidate: get Service by ID:"+service)
	}

	return task, rent, nil
}
