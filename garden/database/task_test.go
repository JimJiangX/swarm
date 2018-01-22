package database

import (
	"errors"
	"testing"
	"time"

	"github.com/docker/swarm/garden/utils"
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

const nullID = "&&&&&&NULL&&&&&"

func makeServiceDesc() ServiceDesc {
	return ServiceDesc{
		ID: nullID,
	}
}

func makeService(status int) Service {
	desc := makeServiceDesc()
	uuid := utils.Generate32UUID()

	return Service{
		Desc:          &desc,
		ID:            uuid,
		Name:          uuid,
		DescID:        desc.ID,
		Tag:           uuid,
		AutoHealing:   false,
		AutoScaling:   true,
		HighAvailable: true,
		Status:        status,
		CreatedAt:     time.Now(),
	}
}

func TestMarkRunningTasks(t *testing.T) {
	if ormer == nil {
		t.Skip("orm:db is required")
	}

	const num = 20
	services := make([]Service, num)
	tasks := make([]Task, num)

	for i := 0; i < num; i++ {
		services[i] = makeService(i<<4 + i%2)
	}

	for i := range services {
		tasks[i] = NewTask(services[i].Name, db.serviceTable(), services[i].ID, services[i].DescID, nil, i<<10)
		tasks[i].Status = i % 3
	}

	prepare := func(tx *sqlx.Tx) error {
		for i := range services {
			err := db.txInsertService(tx, services[i])
			if err != nil {
				return err
			}
		}

		for i := range tasks {
			err := db.txInsertTask(tx, tasks[i], db.serviceTable())
			if err != nil {
				return err
			}
		}

		return nil
	}

	err := ormer.TxFrame(prepare)
	if err != nil {
		t.Fatalf("prepare:%+v", err)
	}

	del := func(tx *sqlx.Tx) error {
		for i := range services {
			err := db.txDelService(tx, services[i].ID)
			if err != nil {
				return err
			}
		}

		return db.delTasks(tasks)
	}

	defer func() {
		err := ormer.TxFrame(del)
		if err != nil {
			t.Fatalf("clean:%+v", err)
		}
	}()

	err = ormer.MarkRunningTasks()
	if err != nil {
		t.Errorf("make:%+v", err)
	}

	tl, err := ormer.ListTasks("", TaskUnknownStatus)
	if err != nil {
		t.Errorf("%+v", err)
	}
	if len(tl) < 7 {
		t.Errorf("expected %d but got %d", 7, len(tl))
	}

	sl, err := db.listServices()
	if err != nil {
		t.Errorf("%+v", err)
	}

	for i := range sl {
		if sl[i].Status&0x0F == 0 {
			t.Log(sl[i].ID, sl[i].Name, sl[i].Status)
		}
	}
}

func TestIncServiceStatus(t *testing.T) {
	if ormer == nil || db == nil {
		t.Skip("orm:db is required")
	}

	tables := make([]Service, 10)

	tasks := make([]Task, 10)

	for i := 0; i < 10; i++ {

		uuid := utils.Generate32UUID()
		now := time.Now()

		tasks[i].Linkto = uuid

		tables[i] = Service{
			ID:         uuid,
			Name:       uuid,
			DescID:     uuid,
			Tag:        uuid,
			Status:     300,
			CreatedAt:  now,
			FinishedAt: now,
		}
	}

	del := func(tx *sqlx.Tx) error {
		for i := range tables {
			err := db.txDelService(tx, tables[i].ID)
			if err != nil {
				return err
			}
		}

		return nil
	}

	defer func() {
		err := ormer.TxFrame(del)
		if err != nil {
			t.Errorf("delete service tests,%+v", err)
		}
	}()

	do := func(tx *sqlx.Tx) error {
		for i := range tables {
			err := db.txInsertService(tx, tables[i])
			if err != nil {
				return err
			}
		}

		err := incServiceStatus(tx, db.serviceTable(), tasks, 1)
		if err != nil {
			return err
		}

		return nil
	}

	err := ormer.TxFrame(do)
	if err != nil {
		t.Errorf("%+v", err)
	}

	for i := range tables {
		status, err := ormer.GetServiceStatus(tables[i].ID)
		if err != nil {
			t.Errorf("%+v", err)
		}
		if status != 301 {
			t.Errorf("expected 301 but got %d", status)
		}
	}
}
