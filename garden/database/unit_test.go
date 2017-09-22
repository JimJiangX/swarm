package database

import (
	"testing"
	"time"

	"github.com/docker/swarm/garden/utils"
	"github.com/jmoiron/sqlx"
)

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
