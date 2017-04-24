package database

import (
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestTask(t *testing.T) {
	if ormer == nil || db == nil {
		t.Skip("orm:db is required")
	}

	out, err := db.ListTasks("", 0)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if len(out) != 0 {
		t.Logf("%d", len(out))
	}

	tasks := make([]Task, 10)
	for i := range tasks {
		tasks[i] = NewTask("object00XXX", ServiceUpdateImageTask, "fjaojfay921084-1hkcshnciaj89r", "ajfakpof90iq3j90qjfoqu89qufjqofu98qufq9fjq9u39qujfof9qjfqj983jq93qiojq",
			map[string]string{
				"abc":             "anfiojaofmaoijaokm fjao a jnoa  j",
				"aaofjafjoajfoaf": "faiojfani naojoamoij jmfojauahincia"}, 3000)
	}

	do := func(tx *sqlx.Tx) error {
		err := db.InsertTasks(tx, tasks[:9], "fjoaufapjioajfoafjceajoapa")
		if err != nil {
			return err
		}

		err = db.txInsertTask(tx, tasks[9], "jafoijaofoajfoa")
		if err != nil {
			return err
		}

		tasks[8].Status = TaskDoneStatus
		tasks[8].errs = nil
		tasks[8].FinishedAt = time.Time{}

		return db.txSetTask(tx, tasks[8])
	}

	err = ormer.TxFrame(do)
	if err != nil {
		t.Errorf("%+v", err)
	}

	tasks[0].Status = TaskDoneStatus
	tasks[0].SetErrors(errors.New("afjoaifemfljoajfeaoji"))

	err = db.SetTask(tasks[0])
	if err != nil {
		t.Errorf("%+v", err)
	}

	out1, err := db.ListTasks("", 0)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if len(out1) != len(out)+10 {
		t.Errorf("%d", len(out1))
	}

	err = db.delTasks(tasks)
	if err != nil {
		t.Errorf("%+v", err)
	}
}
