package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type UnitIface interface {
	GetUnit(nameOrID string) (Unit, error)
	ListUnitByServiceID(id string) ([]Unit, error)
	ListUnitByEngine(id string) ([]Unit, error)

	CountUnitsInEngines(engines []string) (int, error)

	InsertUnits([]Unit) error

	SetUnitInfo(u Unit) error
	UnitStatusCAS(u *Unit, old, value int, operator string) error
	SetUnitWithInsertTask(u *Unit, task Task) error
	SetUnitStatus(u *Unit, status int, msg string) error
	SetUnitAndTask(u *Unit, t *Task, msg string) error
	SetUnits(units []Unit) error
	MigrateUnit(src, destID, destName string) error

	DelUnitsRelated(units []Unit, volume bool) error
}

type ContainerIface interface {
	UnitContainerCreated(name, containerID, engineID, mode string, state int) error

	SetUnitByContainer(containerID string, state int) error
}

type UnitOrmer interface {
	UnitIface

	ContainerIface

	NodeIface

	VolumeOrmer

	NetworkingOrmer

	GetSysConfigIface

	RecycleResource(ips []IP, lvs []Volume) error
}

// Unit is table structure
type Unit struct {
	ID          string `db:"id" json:"id"`
	Name        string `db:"name" json:"name"` // containerName <unit_id_8bit>_<service_name>
	Type        string `db:"type" json:"type"` // switch_manager/upproxy/upsql
	ServiceID   string `db:"service_id" json:"service_id"`
	EngineID    string `db:"engine_id" json:"engine_id"` // engine.ID
	ContainerID string `db:"container_id" json:"container_id"`
	NetworkMode string `db:"network_mode" json:"network_mode"`
	LatestError string `db:"latest_error" json:"latest_error"`

	Status    int       `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (db dbBase) unitTable() string {
	return db.prefix + "_unit"
}

// GetUnit return Unit select by Name or ID or ContainerID
func (db dbBase) GetUnit(nameOrID string) (Unit, error) {
	u := Unit{}
	query := "SELECT id,name,type,service_id,engine_id,container_id,network_mode,latest_error,status,created_at FROM " + db.unitTable() + " WHERE id=? OR name=? OR container_id=?"

	err := db.Get(&u, query, nameOrID, nameOrID, nameOrID)

	return u, errors.Wrap(err, "Get Unit By nameOrID")
}

// InsertUnits insert []Unit in Tx
func (db dbBase) InsertUnits(units []Unit) error {
	do := func(tx *sqlx.Tx) error {
		return db.txInsertUnits(tx, units)
	}

	return db.txFrame(do)
}

// txInsertUnits insert []Unit in Tx
func (db dbBase) txInsertUnits(tx *sqlx.Tx, units []Unit) error {
	if len(units) == 0 {
		return nil
	}

	query := "INSERT INTO " + db.unitTable() + " (id,name,type,service_id,engine_id,container_id,network_mode,latest_error,status,created_at) VALUES (:id,:name,:type,:service_id,:engine_id,:container_id,:network_mode,:latest_error,:status,:created_at)"

	if len(units) == 1 && units[0].ID != "" {
		_, err := tx.NamedExec(query, units[0])

		return errors.Wrap(err, "Tx Insert Unit")
	}

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare insert Unit")
	}

	for i := range units {

		_, err = stmt.Exec(&units[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx Insert Unit")
		}
	}

	stmt.Close()

	return nil
}

func (db dbBase) UnitContainerCreated(name, containerID, engineID, mode string, state int) error {
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	query := "UPDATE " + db.unitTable() + " SET engine_id=?,container_id=?,network_mode=?,status=?,latest_error=? WHERE name=?"
	_, err := db.Exec(query, engineID, containerID, mode, state, "", name)

	return errors.WithStack(err)
}

func (db dbBase) SetUnitByContainer(containerID string, state int) error {

	query := "UPDATE " + db.unitTable() + " SET status=?,latest_error=? WHERE container_id=?"
	_, err := db.Exec(query, state, "", containerID)

	return errors.WithStack(err)
}

// SetUnitInfo could update params of unit
func (db dbBase) SetUnitInfo(u Unit) error {

	query := "UPDATE " + db.unitTable() + " SET name=:name,type=:type,service_id=:service_id,engine_id=:engine_id,container_id=:container_id,network_mode=:network_mode,status=:status,latest_error=:latest_error,created_at=:created_at WHERE id=:id"

	_, err := db.NamedExec(query, &u)

	return errors.Wrap(err, "update Unit info")
}

// txSetUnit upate unit params in tx
func (db dbBase) txSetUnit(tx *sqlx.Tx, u Unit) error {

	query := "UPDATE " + db.unitTable() + " SET engine_id=:engine_id,container_id=:container_id,network_mode=:network_mode,status=:status,latest_error=:latest_error,created_at=:created_at WHERE id=:id"

	_, err := tx.NamedExec(query, &u)

	return errors.Wrap(err, "Tx update Unit")
}

// UnitStatusCAS update Unit Status with conditions,
// Unit status==old or status!=old,update Unit Status to be value if true,else return error
func (db dbBase) UnitStatusCAS(u *Unit, old, value int, operator string) error {
	var (
		status int
		query  = "SELECT status FROM " + db.unitTable() + " WHERE id=?"
	)

	err := db.Get(&status, query, u.ID)
	if err != nil {
		return errors.Wrap(err, "Unit status CAS")
	}
	if status == value {
		return nil
	}

	if operator == "!=" {
		operator = "<>"
	}

	query = fmt.Sprintf("UPDATE %s SET status=? WHERE id=? AND status%s?", db.unitTable(), operator)

	r, err := db.Exec(query, value, u.ID, old)
	if err != nil {
		return errors.Wrap(err, "update Unit Status")
	}

	if n, err := r.RowsAffected(); err != nil || n != 1 {
		return errors.Errorf("unable to update Unit %s,condition:status%s%d", u.ID, operator, old)
	}

	u.Status = value

	return nil
}

// SetUnitWithInsertTask update Unit Status & LatestError and insert Task in Tx
func (db dbBase) SetUnitWithInsertTask(u *Unit, t Task) error {
	do := func(tx *sqlx.Tx) error {
		err := db.txSetUnitStatus(tx, u, u.LatestError, u.Status)
		if err != nil {
			return err
		}

		err = db.txInsertTask(tx, t, db.unitTable())

		return err
	}

	return db.txFrame(do)
}

// SetUnitStatus update Unit Status & LatestError in Tx
func (db dbBase) SetUnitStatus(u *Unit, status int, msg string) error {
	return db.txFrame(
		func(tx *sqlx.Tx) error {
			return db.txSetUnitStatus(tx, u, msg, status)
		})
}

// SetUnitAndTask update Unit and Task in Tx
func (db dbBase) SetUnitAndTask(u *Unit, t *Task, msg string) error {
	do := func(tx *sqlx.Tx) error {
		err := db.txSetUnitStatus(tx, u, msg, u.Status)
		if err != nil {
			return err
		}

		task := *t
		task.Errors = msg
		task.FinishedAt = time.Now()

		err = db.txSetTask(tx, task)

		return err
	}

	err := db.txFrame(do)
	if err == nil {
		u.LatestError = msg
		t.Errors = msg
		t.FinishedAt = time.Now()
	}

	return err
}

func (db dbBase) SetUnits(units []Unit) error {
	do := func(tx *sqlx.Tx) error {
		query := "UPDATE " + db.unitTable() + " SET engine_id=:engine_id,container_id=:container_id,network_mode=:network_mode,status=:status,latest_error=:latest_error,created_at=:created_at WHERE id=:id"

		stmt, err := tx.PrepareNamed(query)
		if err != nil {
			return errors.Wrap(err, "tx prepare")
		}

		for i := range units {

			_, err := stmt.Exec(units[i])
			if err != nil {
				stmt.Close()

				return errors.Wrap(err, "tx update []Unit")
			}
		}

		stmt.Close()

		return nil
	}

	return db.txFrame(do)
}

func (db dbBase) txSetUnitStatus(tx *sqlx.Tx, u *Unit, msg string, status int) error {

	query := "UPDATE " + db.unitTable() + " SET status=?,latest_error=? WHERE id=?"

	_, err := tx.Exec(query, status, msg, u.ID)
	if err != nil {
		return errors.Wrap(err, "Tx update Unit status")
	}

	u.Status = status
	u.LatestError = msg

	return nil
}

// txDelUnit delete Unit by name or ID or ServiceID in Tx
func (db dbBase) txDelUnit(tx *sqlx.Tx, nameOrID string) error {

	query := "DELETE FROM " + db.unitTable() + " WHERE id=? OR name=? OR service_id=?"

	_, err := tx.Exec(query, nameOrID, nameOrID, nameOrID)

	return errors.Wrap(err, "Tx delete Unit by nameOrID or ServiceID")
}

func (db dbBase) listUnits() ([]Unit, error) {
	var (
		out   []Unit
		query = "SELECT id,name,type,service_id,engine_id,container_id,network_mode,latest_error,status,created_at FROM " + db.unitTable()
	)

	err := db.Select(&out, query)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []Unit")
}

// ListUnitByServiceID returns []Unit select by ServiceID
func (db dbBase) ListUnitByServiceID(id string) ([]Unit, error) {
	var (
		out   []Unit
		query = "SELECT id,name,type,service_id,engine_id,container_id,network_mode,latest_error,status,created_at FROM " + db.unitTable() + " WHERE service_id=?"
	)

	err := db.Select(&out, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []Unit by ServiceID")
}

// ListUnitByEngine returns []Unit select by EngineID
func (db dbBase) ListUnitByEngine(id string) ([]Unit, error) {
	var (
		out   []Unit
		query = "SELECT id,name,type,service_id,engine_id,container_id,network_mode,latest_error,status,created_at FROM " + db.unitTable() + " WHERE engine_id=?"
	)

	err := db.Select(&out, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	return out, errors.Wrap(err, "list []Unit by EngineID")
}

// CountUnitByEngine returns len of []Unit select Unit by EngineID
func (db dbBase) CountUnitByEngine(id string) (int, error) {
	count := 0
	query := "SELECT COUNT(id) FROM " + db.unitTable() + " WHERE engine_id=?"

	err := db.Get(&count, query, id)

	return count, errors.Wrap(err, "count Unit by EngineID")
}

// CountUnitsInNodes returns len of []Unit select Unit by NodeID IN Engines.
func (db dbBase) CountUnitsInEngines(engines []string) (int, error) {
	if len(engines) == 0 {
		return 0, nil
	}

	query := "SELECT COUNT(container_id) FROM " + db.unitTable() + " WHERE engine_id IN (?);"

	query, args, err := sqlx.In(query, engines)
	if err != nil {
		return 0, err
	}

	count := 0
	err = db.Get(&count, query, args...)

	return count, errors.Wrap(err, "cound Units by engines")
}

func (db dbBase) MigrateUnit(src, destID, destName string) error {
	do := func(tx *sqlx.Tx) error {
		u := Unit{}
		query := "SELECT id,name,type,service_id,engine_id,container_id,network_mode,latest_error,status,created_at FROM " + db.unitTable() + " WHERE id=? OR name=?"

		err := tx.Get(&u, query, src, src)
		if err != nil {
			return errors.Wrap(err, "Get Unit By nameOrID")
		}

		err = db.txDelVolumeByUnit(tx, destID)
		if err != nil {
			return err
		}

		query = "UPDATE " + db.volumeTable() + " SET unit_id=? WHERE unit_id=?"
		_, err = tx.Exec(query, destID, u.ID)
		if err != nil {
			return errors.Wrap(err, "set volume")
		}

		query = "UPDATE " + db.ipTable() + " SET unit_id=? WHERE unit_id=?"
		_, err = tx.Exec(query, destID, u.ID)
		if err != nil {
			return errors.Wrap(err, "set volume")
		}

		err = db.txDelUnit(tx, destID)
		if err != nil {
			return err
		}

		u.ID = destID
		u.Name = destName
		err = db.txInsertUnits(tx, []Unit{u})
		if err != nil {
			return err
		}

		return db.txDelUnit(tx, src)
	}

	return db.txFrame(do)
}

func (db dbBase) DelUnitsRelated(units []Unit, volume bool) error {
	do := func(tx *sqlx.Tx) error {

		for i := range units {
			u := Unit{}
			query := "SELECT id,name,type,service_id,engine_id,container_id,network_mode,latest_error,status,created_at FROM " + db.unitTable() + " WHERE id=? OR name=? OR container_id=?"

			err := tx.Get(&u, query, units[i].ID, units[i].Name, units[i].ContainerID)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				}
				return errors.Wrap(err, "Get Unit By nameOrID")
			}

			if volume {
				err := db.txDelVolumeByUnit(tx, u.ID)
				if err != nil {
					return err
				}
			}

			err = db.txResetIPByUnit(tx, u.ID)
			if err != nil {
				return err
			}

			err = db.txDelUnit(tx, u.ID)
			if err != nil {
				return err
			}
		}

		return nil
	}

	return db.txFrame(do)
}
