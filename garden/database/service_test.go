package database

import (
	"testing"
	"time"

	"github.com/docker/swarm/garden/utils"
	"github.com/jmoiron/sqlx"
)

func TestListServicesInfo(t *testing.T) {
	if ormer == nil {
		t.Skip("orm:db is required")
	}

	services, err := ormer.ListServicesInfo()
	if err != nil {
		t.Error(err)
	}

	for i := range services {
		t.Logf("%d %+v\n", i, services[i].Service)
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
