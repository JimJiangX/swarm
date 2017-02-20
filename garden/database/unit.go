package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type UnitInterface interface {
	GetUnit(nameOrID string) (Unit, error)
	ListUnitByServiceID(id string) ([]Unit, error)
	ListUnitByEngine(id string) ([]Unit, error)

	CountUnitByEngine(id string) (int, error)
	CountUnitsInEngines(engines []string) (int, error)

	InsertUnit(u Unit) error

	SetUnitInfo(u Unit) error
	UnitStatusCAS(u *Unit, old, value int64, operator string) error
	SetUnitWithInsertTask(u *Unit, task Task) error
	SetUnitStatus(u *Unit, status int64, msg string) error
	SetUnitAndTask(u *Unit, t *Task, msg string) error
	// SetMigrateUnit(u Unit, lvs []Volume, reserveSAN bool) error
}

type ContainerInterface interface {
	UnitContainerCreated(name, containerID, engineID, mode string, state int) error

	SetUnitByContainer(containerID string, state int) error
}

type UnitOrmer interface {
	UnitInterface

	ContainerInterface

	NodeInterface

	VolumeOrmer

	NetworkingOrmer
}

// Unit is table structure
type Unit struct {
	ID          string `db:"id"`
	Name        string `db:"name"` // <unit_id_8bit>_<service_name>
	Type        string `db:"type"` // switch_manager/upproxy/upsql
	ImageID     string `db:"image_id"`
	ImageName   string `db:"image_name"` //<image_name>:<image_version>
	ServiceID   string `db:"service_id"`
	EngineID    string `db:"engine_id"` // engine.ID
	ContainerID string `db:"container_id"`
	ConfigID    string `db:"unit_config_id"`
	NetworkMode string `db:"network_mode"`
	LatestError string `db:"latest_error"`

	Status        int64     `db:"status"`
	CheckInterval int       `db:"check_interval"`
	CreatedAt     time.Time `db:"created_at"`
}

func (db dbBase) unitTable() string {
	return db.prefix + "_unit"
}

// GetUnit return Unit select by Name or ID or ContainerID
func (db dbBase) GetUnit(nameOrID string) (Unit, error) {
	var (
		u     = Unit{}
		query = "SELECT id,name,type,image_id,image_name,service_id,engine_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at FROM " + db.unitTable() + " WHERE id=? OR name=? OR container_id=?"
	)

	err := db.Get(&u, query, nameOrID, nameOrID, nameOrID)

	return u, errors.Wrap(err, "Get Unit By nameOrID")
}

// InsertUnit insert Unit
func (db dbBase) InsertUnit(u Unit) error {

	query := "INSERT INTO " + db.unitTable() + " (id,name,type,image_id,image_name,service_id,engine_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:engine_id,:container_id,:unit_config_id,:network_mode,:status,:latest_error,:check_interval,:created_at)"

	_, err := db.NamedExec(query, &u)

	return errors.Wrap(err, "insert Unit")
}

// txInsertUnit insert Unit in Tx
func (db dbBase) txInsertUnit(tx *sqlx.Tx, u Unit) error {

	query := "INSERT INTO " + db.unitTable() + " (id,name,type,image_id,image_name,service_id,engine_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:engine_id,:container_id,:unit_config_id,:network_mode,:status,:latest_error,:check_interval,:created_at)"

	_, err := tx.NamedExec(query, &u)

	return errors.Wrap(err, "Tx insert Unit")
}

// txInsertUnits insert []Unit in Tx
func (db dbBase) txInsertUnits(tx *sqlx.Tx, units []Unit) error {

	query := "INSERT INTO " + db.unitTable() + " (id,name,type,image_id,image_name,service_id,engine_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:engine_id,:container_id,:unit_config_id,:network_mode,:status,:latest_error,:check_interval,:created_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare insert Unit")
	}

	for i := range units {
		if units[i].ID == "" {
			continue
		}

		_, err = stmt.Exec(units[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx Insert Unit")
		}
	}

	err = stmt.Close()

	return errors.Wrap(err, "insert []*Unit")
}

func (db dbBase) UnitContainerCreated(name, containerID, engineID, mode string, state int) error {
	query := "UPDATE " + db.unitTable() + " SET engine_id=?,container_id=?,network_mode=?,status=?,latest_error=? WHERE name=?"
	_, err := db.Exec(query, engineID, containerID, mode, state, "", name)

	return err
}

func (db dbBase) SetUnitByContainer(containerID string, state int) error {
	query := "UPDATE " + db.unitTable() + " SET status=?,latest_error=? WHERE container_id=?"
	_, err := db.Exec(query, state, "", containerID)

	return err
}

// SetUnitInfo could update params of unit
func (db dbBase) SetUnitInfo(u Unit) error {

	query := "UPDATE " + db.unitTable() + " SET name=:name,type=:type,image_id=:image_id,image_name=:image_name,service_id=:service_id,engine_id=:engine_id,container_id=:container_id,unit_config_id=:unit_config_id,network_mode=:network_mode,status=:status,latest_error=:latest_error,check_interval=:check_interval,created_at=:created_at WHERE id=:id"

	_, err := db.NamedExec(query, &u)

	return errors.Wrap(err, "update Unit params")
}

// txSetUnit upate unit params in tx
func (db dbBase) txSetUnit(tx *sqlx.Tx, u Unit) error {

	query := "UPDATE " + db.unitTable() + " SET engine_id=:engine_id,container_id=:container_id,status=:status,latest_error=:latest_error,created_at=:created_at WHERE id=:id"

	_, err := tx.NamedExec(query, u)

	return errors.Wrap(err, "Tx update Unit")
}

// UnitStatusCAS update Unit Status with conditions,
// Unit status==old or status!=old,update Unit Status to be value if true,else return error
func (db dbBase) UnitStatusCAS(u *Unit, old, value int64, operator string) error {
	var (
		status int64
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
func (db dbBase) SetUnitWithInsertTask(u *Unit, task Task) error {
	do := func(tx *sqlx.Tx) error {
		err := db.txSetUnitStatus(tx, u, u.Status, u.LatestError)
		if err != nil {
			return err
		}

		err = db.txInsertTask(tx, task)

		return err
	}

	return db.txFrame(do)
}

// SetUnitStatus update Unit Status & LatestError in Tx
func (db dbBase) SetUnitStatus(u *Unit, status int64, msg string) error {
	return db.txFrame(
		func(tx *sqlx.Tx) error {
			return db.txSetUnitStatus(tx, u, status, msg)
		})
}

// SetUnitAndTask update Unit and Task in Tx
func (db dbBase) SetUnitAndTask(u *Unit, t *Task, msg string) error {
	do := func(tx *sqlx.Tx) error {
		err := db.txSetUnitStatus(tx, u, u.Status, u.LatestError)
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
		t.Errors = msg
		t.FinishedAt = time.Now()
	}

	return err
}

func (db dbBase) txSetUnitStatus(tx *sqlx.Tx, u *Unit, status int64, msg string) error {

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
		query = "SELECT id,name,type,image_id,image_name,service_id,engine_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at FROM " + db.unitTable()
	)

	err := db.Select(&out, query)

	return out, errors.Wrap(err, "list []Unit")
}

// ListUnitByServiceID returns []Unit select by ServiceID
func (db dbBase) ListUnitByServiceID(id string) ([]Unit, error) {
	var (
		out   []Unit
		query = "SELECT id,name,type,image_id,image_name,service_id,engine_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at FROM " + db.unitTable() + " WHERE service_id=?"
	)

	err := db.Select(&out, query, id)

	return out, errors.Wrap(err, "list []Unit by ServiceID")
}

// ListUnitByEngine returns []Unit select by EngineID
func (db dbBase) ListUnitByEngine(id string) ([]Unit, error) {
	var (
		out   []Unit
		query = "SELECT id,name,type,image_id,image_name,service_id,engine_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at FROM " + db.unitTable() + " WHERE engine_id=?"
	)

	err := db.Select(&out, query, id)

	return out, errors.Wrap(err, "list []Unit by EngineID")
}

// CountUnitByEngine returns len of []Unit select Unit by EngineID
func (db dbBase) CountUnitByEngine(id string) (int, error) {
	var (
		count = 0
		query = "SELECT COUNT(id) FROM " + db.unitTable() + " WHERE engine_id=?"
	)

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

// TxInsertUnitWithPorts insert Unit and update []Port in a Tx
func (db dbBase) InsertUnitWithPorts(u *Unit, ports []Port) error {
	do := func(tx *sqlx.Tx) (err error) {
		if u != nil {
			err = db.txInsertUnit(tx, *u)
			if err != nil {
				return err
			}
		}

		err = db.txSetPorts(tx, ports)

		return err
	}

	return db.txFrame(do)
}

// SetMigrateUnit update Unit and delete old LocalVolumes in a Tx
func (db dbBase) SetMigrateUnit(u Unit, lvs []Volume, reserveSAN bool) error {
	// update database Unit
	// delete old localVolumes
	do := func(tx *sqlx.Tx) error {
		for i := range lvs {
			if reserveSAN && strings.HasSuffix(lvs[i].VGName, "_SAN_VG") {
				continue
			}

			err := db.txDelVolume(tx, lvs[i].ID)
			if err != nil {
				return err
			}
		}

		return db.txSetUnit(tx, u)
	}

	return db.txFrame(do)
}

//// SaveUnitConfig insert UnitConfig and update Unit.ConfigID in Tx
//func (db dbBase) SaveUnitConfig(unit *Unit, config UnitConfig) error {
//	do := func(tx *sqlx.Tx) (err error) {

//		if unit != nil && unit.ID != "" {
//			query := "UPDATE " + db.unitTable() + " SET unit_config_id=? WHERE id=?"

//			_, err = tx.Exec(query, config.ID, unit.ID)
//			if err != nil {
//				return errors.Wrap(err, "Tx Update Unit ConfigID")
//			}
//		}

//		config.UnitID = unit.ID

//		err = db.TXInsertUnitConfig(tx, &config)

//		return err
//	}

//	err := db.txFrame(do)
//	if err == nil {
//		unit.ConfigID = config.ID
//	}

//	return err
//}
