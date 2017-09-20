package database

import (
	"database/sql"
	"time"

	"bytes"

	"github.com/docker/swarm/garden/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const (
	_ = iota // 0
	// TaskCreateStatus task is created
	TaskCreateStatus // 1
	// TaskRunningStatus task is running
	TaskRunningStatus // 2
	// TaskStopStatus task is stoped
	TaskStopStatus // 3
	// TaskCancelStatus task is canceled
	TaskCancelStatus // 4
	// TaskDoneStatus task has done
	TaskDoneStatus // 5
	// TaskTimeoutStatus task is timeout and stoped
	TaskTimeoutStatus // 6
	// TaskFailedStatus task is failed
	TaskFailedStatus // 7
	// TaskUnknownStatus task status change by replicate changes
	TaskUnknownStatus // 8
)

const (
	// related to Task.Related

	// node distribution task
	NodeInstall = "host_install"

	// load image task
	ImageLoadTask = "image_load"

	// create and run service task
	ServiceRunTask    = "service_create"
	ServiceLinkTask   = "services_link"
	ServiceStartTask  = "service_start"
	ServiceStopTask   = "service_stop"
	ServiceScaleTask  = "service_scale"
	ServiceUpdateTask = "service_update"
	ServiceExecTask   = "service_exec"
	ServiceBackupTask = "service_backup"

	ServiceUpdateConfigTask = "service_update_config"
	ServiceUpdateImageTask  = "service_update_image"

	// unit tasks
	UnitMigrateTask = "unit_migrate"
	UnitRebuildTask = "unit_rebuild"
	UnitRestoreTask = "unit_restore"

	// backup tasks
	BackupAutoTask   = "backup_auto"
	BackupManualTask = "backup_manual"
)

// TaskOrmer Task db table operators
type TaskOrmer interface {
	InsertTask(t Task) error

	InsertTasks(tx *sqlx.Tx, tasks []Task, linkTable string) error

	GetTask(ID string) (Task, error)

	ListTasks(link string, status int) ([]Task, error)

	SetTask(t Task) error
}

// NewTask new a Task
func NewTask(object, relate, linkto, desc string, label map[string]string, timeout int) Task {
	tk := task{
		ID:        utils.Generate32UUID(),
		Name:      relate + "-" + object,
		Related:   relate,
		Linkto:    linkto,
		Desc:      desc,
		Timeout:   time.Duration(timeout) * time.Second,
		Status:    TaskRunningStatus,
		CreatedAt: time.Now(),
	}

	t := Task{task: tk, label: label}
	t.toTask()

	return t
}

// Task

type Task struct {
	task
	errs  []error
	label map[string]string
}

func (t *Task) SetErrors(err ...error) {
	t.errs = err
}

func (t *Task) AddErr(err error) {
	if t.errs != nil {
		t.errs = append(t.errs, err)
	} else {
		t.errs = []error{err}
	}
}

func (t *Task) toTask() task {
	if len(t.errs) > 0 {
		buf := bytes.NewBuffer(nil)
		for i := range t.errs {
			if t.errs[i] != nil {
				buf.WriteString(t.errs[i].Error())
				buf.WriteByte('\n')
			}
		}
		t.Errors = buf.String()
		t.errs = nil
	}

	if len(t.label) > 0 {
		buf := bytes.NewBufferString(t.Labels)
		for k, v := range t.label {
			buf.WriteString(k)
			buf.WriteByte(':')
			buf.WriteString(v)
			buf.WriteByte('\n')
		}

		t.Labels = buf.String()
		t.label = nil
	}

	if !t.CreatedAt.IsZero() {
		t.Timestamp = t.CreatedAt.Unix()
	}

	return t.task
}

// task is table structure,record tasks status
type task struct {
	ID         string        `db:"id" json:"id"`
	Name       string        `db:"name" json:"name"` //Related-Object
	Related    string        `db:"related" json:"related"`
	Linkto     string        `db:"link_to" json:"link_to"`
	LinkTable  string        `db:"link_table" json:"-"`
	Desc       string        `db:"description" json:"description"`
	Labels     string        `db:"labels" json:"labels"`
	Errors     string        `db:"errors" json:"errors"`
	Status     int           `db:"status" json:"status"`
	Timestamp  int64         `db:"timestamp" json:"timestamp"` // time.Time.Unix()
	Timeout    time.Duration `db:"timeout" json:"timeout"`
	CreatedAt  time.Time     `db:"created_at" json:"created_at"`
	FinishedAt time.Time     `db:"finished_at" json:"finished_at"`
}

func (db dbBase) taskTable() string {
	return db.prefix + "_task"
}

func (db dbBase) InsertTask(t Task) error {
	tk := t.toTask()

	query := "INSERT INTO " + db.taskTable() + " (id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:link_table,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

	_, err := db.NamedExec(query, tk)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "insert Task")

}

func (db dbBase) txInsertTask(tx *sqlx.Tx, t Task, linkTable string) error {
	if t.LinkTable == "" {
		t.LinkTable = linkTable
	}

	tk := t.toTask()

	query := "INSERT INTO " + db.taskTable() + " (id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:link_table,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

	_, err := tx.NamedExec(query, tk)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx insert Task")
}

func (db dbBase) InsertTasks(tx *sqlx.Tx, tasks []Task, linkTable string) error {
	if len(tasks) == 1 {
		return db.txInsertTask(tx, tasks[0], linkTable)
	}

	query := "INSERT INTO " + db.taskTable() + " (id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:link_table,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "tx prepare insert task")
	}

	for i := range tasks {
		if tasks[i].ID == "" {
			continue
		}

		if tasks[i].LinkTable == "" {
			tasks[i].LinkTable = linkTable
		}

		tk := tasks[i].toTask()

		_, err = stmt.Exec(tk)
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx insert []Task")
		}
	}

	stmt.Close()

	return nil
}

func (db dbBase) txSetTask(tx *sqlx.Tx, t Task) error {
	tk := t.toTask()

	query := "UPDATE " + db.taskTable() + " SET status=?,finished_at=?,errors=? WHERE id=?"

	_, err := tx.Exec(query, tk.Status, tk.FinishedAt, tk.Errors, tk.ID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx update Task status & errors")
}

func (db dbBase) SetTask(t Task) error {
	if t.FinishedAt.IsZero() {
		t.FinishedAt = time.Now()
	}

	tk := t.toTask()

	query := "UPDATE " + db.taskTable() + " SET status=?,finished_at=?,errors=? WHERE id=?"

	_, err := db.Exec(query, tk.Status, tk.FinishedAt, tk.Errors, tk.ID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "update Task status & errors")
}

func (db dbBase) GetTask(ID string) (Task, error) {
	tk := task{}
	query := "SELECT id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM " + db.taskTable() + " WHERE id=?"

	err := db.Get(&tk, query, ID)
	if err == nil {
		return Task{task: tk}, nil
	}

	return Task{}, errors.Wrap(err, "get task by id:"+ID)
}

func (db dbBase) ListTasks(link string, status int) ([]Task, error) {
	var (
		err   error
		tks   []task
		query = "SELECT id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM " + db.taskTable()
	)

	switch {
	case status > 0:
		query = query + " WHERE status=?"

		err = db.Select(&tks, query, status)

	case link != "":

		query = query + " WHERE link_to=?"

		err = db.Select(&tks, query, link)

	default:

		err = db.Select(&tks, query)
	}

	if err == nil {
		out := make([]Task, 0, len(tks))
		for i := range tks {
			out = append(out, Task{task: tks[i]})
		}

		return out, nil

	} else if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, errors.Wrap(err, "list tasks")
}

func (db dbBase) txListTasks(tx *sqlx.Tx, status int) ([]Task, error) {
	var (
		err   error
		tks   []task
		query = "SELECT id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM " + db.taskTable() + " WHERE status=?"
	)

	err = tx.Select(&tks, query, status)

	if err == nil {
		out := make([]Task, 0, len(tks))
		for i := range tks {
			out = append(out, Task{task: tks[i]})
		}

		return out, nil

	} else if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, errors.Wrap(err, "tx list tasks")
}

func (db dbBase) delTasks(tasks []Task) error {
	stmt, err := db.Preparex("DELETE FROM " + db.taskTable() + " WHERE id=?")
	if err != nil {
		return errors.WithStack(err)
	}

	for i := range tasks {
		_, err = stmt.Exec(tasks[i].ID)
		if err != nil {
			stmt.Close()
			return errors.WithStack(err)
		}
	}

	stmt.Close()

	return nil
}
