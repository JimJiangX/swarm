package database

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type TaskOrmer interface {
	InsertTasks(tx *sqlx.Tx, tasks []Task) error
}

func NewTask() Task {
	return Task{}
}

// Task is table structure,record tasks status
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

func (db dbBase) taskTable() string {
	return db.prefix + "_task"
}

func (db dbBase) txInsertTask(tx *sqlx.Tx, t Task) error {
	query := "INSERT INTO " + db.taskTable() + " (id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

	_, err := tx.Exec(query, t)

	return errors.Wrap(err, "Tx insert Task")
}

func (db dbBase) InsertTasks(tx *sqlx.Tx, tasks []Task) error {
	if len(tasks) == 1 {
		return db.txInsertTask(tx, tasks[0])
	}

	query := "INSERT INTO " + db.taskTable() + " (id,name,related,link_to,description,labels,errors,timeout,status,created_at,timestamp,finished_at) VALUES (:id,:name,:related,:link_to,:description,:labels,:errors,:timeout,:status,:created_at,:timestamp,:finished_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "tx prepare insert task")
	}

	for i := range tasks {
		if tasks[i].ID == "" {
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
