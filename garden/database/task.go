package database

import (
	"database/sql"
	"time"

	"github.com/docker/swarm/garden/utils"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const (
	_                 = iota // 0
	TaskCreateStatus         // 1
	TaskRunningStatus        // 2
	TaskStopStatus           // 3
	TaskCancelStatus         // 4
	TaskDoneStatus           // 5
	TaskTimeoutStatus        // 6
	TaskFailedStatus         // 7
)

const (
	// related to Task.Related

	// node distribution task
	NodeInstall = "host_install"

	// load image task
	ImageLoadTask = "image_load"

	// create and run service task
	ServiceRunTask  = "service_create"
	ServiceLinkTask = "Services_link"

	// unit tasks
	UnitMigrateTask = "unit_migrate"
	UnitRebuildTask = "unit_rebuild"
	UnitRestoreTask = "unit_restore"

	// backup tasks
	BackupAutoTask   = "backup_auto"
	BackupManualTask = "backup_manual"
)

type TaskOrmer interface {
	InsertTasks(tx *sqlx.Tx, tasks []Task, linkTable string) error

	GetTask(ID string) (Task, error)

	ListTasks(link string, status int) ([]Task, error)

	SetTask(t Task) error
}

// NewTask new a Task
func NewTask(object, relate, linkto, desc, labels string, timeout int) Task {
	return Task{
		ID:        utils.Generate32UUID(),
		Name:      relate + "-" + object,
		Related:   relate,
		Linkto:    linkto,
		Desc:      desc,
		Labels:    labels,
		Timeout:   time.Duration(timeout) * time.Second,
		Status:    TaskRunningStatus,
		CreatedAt: time.Now(),
	}
}

// Task is table structure,record tasks status
type Task struct {
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

func (db dbBase) txInsertTask(tx *sqlx.Tx, t Task, linkTable string) error {
	if t.LinkTable == "" {
		t.LinkTable = linkTable
	}
	t.Timestamp = t.CreatedAt.Unix()

	query := "INSERT INTO " + db.taskTable() + " (id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:link_table,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

	_, err := tx.NamedExec(query, &t)

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
		tasks[i].Timestamp = tasks[i].CreatedAt.Unix()

		_, err = stmt.Exec(&tasks[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx insert []Task")
		}
	}

	err = stmt.Close()

	return errors.Wrap(err, "Tx insert []Task")
}

func (db dbBase) txSetTask(tx *sqlx.Tx, t Task) error {

	query := "UPDATE " + db.taskTable() + " SET status=?,finished_at=?,errors=? WHERE id=?"

	_, err := tx.Exec(query, t.Status, t.FinishedAt, t.Errors, t.ID)
	if err != nil {
		return errors.Wrap(err, "Tx update Task status & errors")
	}

	return nil
}

func (db dbBase) SetTask(t Task) error {
	if t.FinishedAt.IsZero() {
		t.FinishedAt = time.Now()
	}

	query := "UPDATE " + db.taskTable() + " SET status=?,finished_at=?,errors=? WHERE id=?"

	_, err := db.Exec(query, t.Status, t.FinishedAt, t.Errors, t.ID)
	if err != nil {
		return errors.Wrap(err, "Tx update Task status & errors")
	}

	return nil
}

func (db dbBase) GetTask(ID string) (t Task, err error) {
	query := "SELECT id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM " + db.taskTable() + " WHERE id=?"

	err = db.Get(&t, query, ID)

	return t, errors.Wrap(err, "get task by id:"+ID)
}

func (db dbBase) ListTasks(link string, status int) ([]Task, error) {
	var (
		err   error
		out   []Task
		query = "SELECT id,name,related,link_to,link_table,description,labels,errors,timeout,status,created_at,timestamp,finished_at FROM " + db.taskTable()
	)

	switch {
	case status > 0:
		query = query + " WHERE status=?"

		err = db.Select(&out, query, status)

	case link != "":

		query = query + " WHERE link_to=?"

		err = db.Select(&out, query, link)

	default:

		err = db.Select(&out, query)
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list tasks")
}
