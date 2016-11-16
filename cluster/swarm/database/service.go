package database

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

/*
type Container struct {
	ID             string    `db:"id"`
	NodeID         string    `db:"node_id"`
	NetworkingID   string    `db:"networking_id"`
	IPAddr         string    `db:"ip_addr"`
	Ports          string    `db:"ports"`
	Image          string    `db:"image"`
	CPUSet         string    `db:"cpu_set"`
	CPUShares      int64     `db:"cpu_shares"`
	NCPU           int       `db:"ncpu"`
	MemoryByte     int64     `db:"mem"`
	MemorySwapByte int64     `db:"mem_swap"`
	StorageType    string    `db:"storage_type"`
	NetworkMode    string    `db:"network_mode"`
	VolumeDriver   string    `db:"volume_driver"`
	VolumesFrom    string    `db:"volumes_from"` // JSON of volumes
	Filesystem     string    `db:"filesystem"`
	Env            string    `db:"env"`
	Cmd            string    `db:"cmd"`
	Labels         string    `db:"labels"`
	CreatedAt      time.Time `db:"create_at"`
}

func (c Container) tableName() string {
	return "tbl_dbaas_container"
}

func TxInsertMultiContainer(tx *sqlx.Tx, clist []*Container) error {
	query := "INSERT INTO tbl_dbaas_container (id,node_id,networking_id,ip_addr,ports,image,cpu_set,cpu_shares,ncpu,mem,mem_swap,storage_type,network_mode,volume_driver,volumes_from,filesystem,env,cmd,labels,create_at) VALUES (:id,:node_id,:networking_id,:ip_addr,:ports,:image,:cpu_set,:cpu_shares,:ncpu,:mem,:mem_swap,:storage_type,:network_mode,:volume_driver,:volumes_from,:filesystem,:env,:cmd,:labels,:create_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range clist {

		if clist[i] == nil {
			continue
		}

		_, err = stmt.Exec(clist[i])
		if err != nil {
			return err
		}
	}

	return nil
}
*/

const insertUnitQuery = "INSERT INTO tbl_dbaas_unit (id,name,type,image_id,image_name,service_id,node_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:node_id,:container_id,:unit_config_id,:network_mode,:status,:latest_error,:check_interval,:created_at)"

// Unit is table tbl_dbaas_unit structure
type Unit struct {
	ID          string `db:"id"`
	Name        string `db:"name"` // <unit_id_8bit>_<service_name>
	Type        string `db:"type"` // switch_manager/upproxy/upsql
	ImageID     string `db:"image_id"`
	ImageName   string `db:"image_name"` //<image_name>:<image_version>
	ServiceID   string `db:"service_id"`
	EngineID    string `db:"node_id"` // engine.ID
	ContainerID string `db:"container_id"`
	ConfigID    string `db:"unit_config_id"`
	NetworkMode string `db:"network_mode"`
	LatestError string `db:"latest_error"`

	Status        int64     `db:"status"`
	CheckInterval int       `db:"check_interval"`
	CreatedAt     time.Time `db:"created_at"`
}

func (u Unit) tableName() string {
	return "tbl_dbaas_unit"
}

// GetUnit return Unit select by Name or ID or ContainerID
func GetUnit(nameOrID string) (Unit, error) {
	u := Unit{}

	db, err := getDB(false)
	if err != nil {
		return u, err
	}

	const query = "SELECT * FROM tbl_dbaas_unit WHERE id=? OR name=? OR container_id=?"

	err = db.Get(&u, query, nameOrID, nameOrID, nameOrID)
	if err == nil {
		return u, nil
	}

	db, err = getDB(true)
	if err != nil {
		return u, err
	}

	err = db.Get(&u, query, nameOrID, nameOrID, nameOrID)

	return u, errors.Wrap(err, "Get Unit By nameOrID")
}

// txInsertUnit insert Unit in Tx
func txInsertUnit(tx *sqlx.Tx, unit Unit) error {
	_, err := tx.NamedExec(insertUnitQuery, &unit)

	return errors.Wrap(err, "Tx insert Unit")
}

// InsertUnit insert Unit
func InsertUnit(u Unit) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertUnitQuery, &u)

	return errors.Wrap(err, "insert Unit")
}

// UpdateUnitInfo could update params of unit
func UpdateUnitInfo(unit Unit) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_unit SET name=:name,type=:type,image_id=:image_id,image_name=:image_name,service_id=:service_id,node_id=:node_id,container_id=:container_id,unit_config_id=:unit_config_id,network_mode=:network_mode,status=:status,latest_error=:latest_error,check_interval=:check_interval,created_at=:created_at WHERE id=:id"

	_, err = db.NamedExec(query, &unit)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(query, &unit)

	return errors.Wrap(err, "update Unit params")
}

// txUpdateUnit upate unit params in tx
func txUpdateUnit(tx *sqlx.Tx, unit Unit) error {
	const query = "UPDATE tbl_dbaas_unit SET node_id=:node_id,container_id=:container_id,status=:status,latest_error=:latest_error,created_at=:created_at WHERE id=:id"

	_, err := tx.NamedExec(query, unit)

	return errors.Wrap(err, "Tx update Unit")
}

// StatusCAS update Unit Status with conditions,
// Unit status==old or status!=old,update Unit Status to be value if true,else return error
func (u *Unit) StatusCAS(operator string, old, value int64) error {
	if operator == "!=" {
		operator = "<>"
	}

	query := fmt.Sprintf("UPDATE tbl_dbaas_unit SET status=? WHERE id=? AND status%s?", operator)

	db, err := getDB(true)
	if err != nil {
		return err
	}

	var status int64
	err = db.Get(&status, "SELECT status FROM tbl_dbaas_unit WHERE id=?", u.ID)
	if err != nil {
		return errors.Wrap(err, "Unit status CAS")
	}
	if status == value {
		return nil
	}

	r, err := db.Exec(query, value, u.ID, old)
	if err != nil {
		return errors.Wrap(err, "update Unit Status")
	}

	if n, err := r.RowsAffected(); err != nil || n != 1 {
		return errors.Errorf("unable to update Unit %s,condition:status%s%d", u.ID, operator, old)
	}

	atomic.StoreInt64(&u.Status, value)

	return nil
}

// TxUpdateUnitAndInsertTask update Unit Status & LatestError and insert Task in Tx
func TxUpdateUnitAndInsertTask(unit *Unit, task Task) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txUpdateUnitStatus(tx, unit, unit.Status, unit.LatestError)
	if err != nil {
		return err
	}

	err = TxInsertTask(tx, task)
	if err != nil {
		return err
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx update Unit and insert Task")
}

// TxUpdateUnitStatus update Unit Status & LatestError in Tx
func TxUpdateUnitStatus(unit *Unit, status int64, msg string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txUpdateUnitStatus(tx, unit, status, msg)
	if err != nil {
		return err
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx update Unit Status & LatestError")
}

// TxUpdateUnitStatusWithTask update Unit and Task in Tx
func TxUpdateUnitStatusWithTask(unit *Unit, task *Task, msg string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txUpdateUnitStatus(tx, unit, unit.Status, unit.LatestError)
	if err != nil {
		return err
	}

	err = txUpdateTaskStatus(tx, task, task.Status, time.Now(), msg)
	if err != nil {
		return err
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx update Unit & Task")
}

func txUpdateUnitStatus(tx *sqlx.Tx, unit *Unit, status int64, msg string) error {
	_, err := tx.Exec("UPDATE tbl_dbaas_unit SET status=?,latest_error=? WHERE id=?", status, msg, unit.ID)
	if err != nil {
		return errors.Wrap(err, "Tx update Unit status")
	}

	atomic.StoreInt64(&unit.Status, status)
	unit.LatestError = msg

	return nil
}

// TxDeleteUnit delete Unit by name or ID or ServiceID in Tx
func TxDeleteUnit(tx *sqlx.Tx, nameOrID string) error {
	_, err := tx.Exec("DELETE FROM tbl_dbaas_unit WHERE id=? OR name=? OR service_id=?", nameOrID, nameOrID, nameOrID)

	return errors.Wrap(err, "Tx delete Unit by nameOrID or ServiceID")
}

// ListUnitByServiceID returns []Unit select by ServiceID
func ListUnitByServiceID(id string) ([]Unit, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var out []Unit
	const query = "SELECT * FROM tbl_dbaas_unit WHERE service_id=?"

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, id)

	return out, errors.Wrap(err, "list []Unit by ServiceID")
}

// ListUnitByEngine returns []Unit select by EngineID
func ListUnitByEngine(id string) ([]Unit, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var out []Unit
	const query = "SELECT * FROM tbl_dbaas_unit WHERE node_id=?"

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, id)

	return out, errors.Wrap(err, "list []Unit by EngineID")
}

// CountUnitByNode returns len of []Unit select Unit by EngineID
func CountUnitByNode(id string) (int, error) {
	db, err := getDB(false)
	if err != nil {
		return 0, err
	}

	count := 0
	const query = "SELECT COUNT(id) from tbl_dbaas_unit WHERE node_id=?"

	err = db.Get(&count, query, id)
	if err == nil {
		return count, nil
	}

	db, err = getDB(true)
	if err != nil {
		return 0, err
	}

	err = db.Get(&count, query, id)

	return count, errors.Wrap(err, "count Unit by NodeID")
}

// CountUnitsInNodes returns len of []Unit select Unit by NodeID IN Engines.
func CountUnitsInNodes(engines []string) (int, error) {
	if len(engines) == 0 {
		return 0, nil
	}

	db, err := getDB(true)
	if err != nil {
		return 0, err
	}

	query, args, err := sqlx.In("SELECT COUNT(container_id) FROM tbl_dbaas_unit WHERE node_id IN (?);", engines)
	if err != nil {
		return 0, err
	}

	count := 0
	err = db.Get(&count, query, args...)

	return count, errors.Wrap(err, "cound Units by engines")
}

// SaveUnitConfig insert UnitConfig and update Unit.ConfigID in Tx
func SaveUnitConfig(unit *Unit, config UnitConfig) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if unit != nil && unit.ID != "" {
		const query = "UPDATE tbl_dbaas_unit SET unit_config_id=? WHERE id=?"

		_, err = tx.Exec(query, config.ID, unit.ID)
		if err != nil {
			return errors.Wrap(err, "Tx Update Unit ConfigID")
		}
	}

	config.UnitID = unit.ID

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "save Unit Config")
	}

	unit.ConfigID = config.ID

	return nil
}

const insertServiceQuery = "INSERT INTO tbl_dbaas_service (id,name,description,architecture,business_code,auto_healing,auto_scaling,status,backup_max_size,backup_files_retention,created_at,finished_at) VALUES (:id,:name,:description,:architecture,:business_code,:auto_healing,:auto_scaling,:status,:backup_max_size,:backup_files_retention,:created_at,:finished_at)"

// Service if table tbl_dbaas_service structure
type Service struct {
	ID           string `db:"id"`
	Name         string `db:"name"`
	Desc         string `db:"description"` // short for Description
	Architecture string `db:"architecture"`
	BusinessCode string `db:"business_code"`
	AutoHealing  bool   `db:"auto_healing"`
	AutoScaling  bool   `db:"auto_scaling"`
	//	HighAvailable     bool   `db:"high_available"`
	Status            int64 `db:"status"`
	BackupMaxSizeByte int   `db:"backup_max_size"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	BackupFilesRetention int       `db:"backup_files_retention"`
	CreatedAt            time.Time `db:"created_at"`
	FinishedAt           time.Time `db:"finished_at"`
}

func (svc Service) tableName() string {
	return "tbl_dbaas_service"
}

// ListServices returns all []Service
func ListServices() ([]Service, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var out []Service
	const query = "SELECT * FROM tbl_dbaas_service"

	err = db.Select(&out, query)
	if err == nil {
		return out, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query)

	return out, errors.Wrap(err, "list []Service")
}

// GetService returns Service select by ID or Name
func GetService(nameOrID string) (Service, error) {
	db, err := getDB(false)
	if err != nil {
		return Service{}, err
	}

	s := Service{}
	const query = "SELECT * FROM tbl_dbaas_service WHERE id=? OR name=?"

	err = db.Get(&s, query, nameOrID, nameOrID)
	if err == nil {
		return s, nil
	}

	db, err = getDB(true)
	if err != nil {
		return Service{}, err
	}

	err = db.Get(&s, query, nameOrID, nameOrID)

	return s, errors.Wrap(err, "get Service by nameOrID")
}

// GetServiceStatus returns Service Status select by ID or Name
func GetServiceStatus(nameOrID string) (int, error) {
	db, err := getDB(false)
	if err != nil {
		return 0, err
	}

	var n int
	const query = "SELECT status FROM tbl_dbaas_service WHERE id=? OR name=?"

	err = db.Get(&n, query, nameOrID, nameOrID)

	return n, errors.Wrap(err, "get Service.Status by nameOrID")
}

func TxServiceStatusCAS(nameOrID string, val int, finish time.Time, f func(val int) bool) (bool, int, error) {
	tx, err := GetTX()
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback()

	var n int
	const query = "SELECT status FROM tbl_dbaas_service WHERE id=? OR name=?"

	err = tx.Get(&n, query, nameOrID, nameOrID)
	if err != nil {
		return false, 0, errors.Wrap(err, "Tx get Service Status")
	}

	if !f(n) {
		return false, n, nil
	}

	_, err = tx.Exec("UPDATE tbl_dbaas_service SET status=?,finished_at=? WHERE id=? OR name=?", val, finish, nameOrID, nameOrID)
	if err != nil {
		return false, val, errors.Wrap(err, "Tx update Service status")
	}

	err = tx.Commit()
	if err == nil {
		return true, val, nil
	}

	return false, n, errors.Wrap(err, "Tx Service Status CAS")
}

func UpdateServiceStatus(nameOrID string, val int, finish time.Time) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tbl_dbaas_service SET status=?,finished_at=? WHERE id=? OR name=?", val, finish, nameOrID, nameOrID)

	return errors.Wrap(err, "update Service Status")
}

// TxGetServiceByUnit returns Service select by Unit ID or Name.
func TxGetServiceByUnit(unit string) (Service, error) {
	tx, err := GetTX()
	if err != nil {
		return Service{}, err
	}
	defer tx.Rollback()

	const (
		queryUnit    = "SELECT service_id FROM tbl_dbaas_unit WHERE id=? OR name=?"
		queryService = "SELECT * FROM tbl_dbaas_service WHERE id=?"
	)

	var (
		id      string
		service Service
	)

	err = tx.Get(&id, queryUnit, unit, unit)
	if err != nil {
		return Service{}, errors.Wrap(err, "Tx get Unit")
	}

	err = tx.Get(&service, queryService, id)
	if err != nil {
		return Service{}, errors.Wrap(err, "Tx get Service")
	}

	err = tx.Commit()

	return service, errors.Wrap(err, "Tx get Service by unit")
}

// UpdateServcieDesc update Service Description
func UpdateServcieDesc(id, desc string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_service SET description=? WHERE id=?"

	_, err = db.Exec(query, desc, id)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, desc, id)

	return errors.Wrap(err, "update Service.Desc")
}

// TxSaveService insert Service & BackupStrategy & Task & []User in Tx.
func TxSaveService(svc Service, strategy *BackupStrategy, task *Task, users []User) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txInsertSerivce(tx, svc)
	if err != nil {
		return err
	}

	if task != nil {
		err = TxInsertTask(tx, *task)
		if err != nil {
			return err
		}
	}

	if strategy != nil {
		err = TxInsertBackupStrategy(tx, *strategy)
		if err != nil {
			return err
		}
	}

	if len(users) > 0 {
		err = txInsertUsers(tx, users)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx save Service & BackupStrategy & Task & []User")
}

func txInsertSerivce(tx *sqlx.Tx, svc Service) error {
	_, err := tx.NamedExec(insertServiceQuery, &svc)

	return errors.Wrap(err, "Tx insert Service")
}

// TxSetServiceStatus update Service Status and Task Status in Tx.
func TxSetServiceStatus(svc *Service, task *Task, state, tstate int64, finish time.Time, msg string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if finish.IsZero() {
		_, err := tx.Exec("UPDATE tbl_dbaas_service SET status=? WHERE id=?", state, svc.ID)
		if err != nil {
			return errors.Wrap(err, "Tx update Service status")
		}
	} else {
		_, err := tx.Exec("UPDATE tbl_dbaas_service SET status=?,finished_at=? WHERE id=?", state, finish, svc.ID)
		if err != nil {
			return errors.Wrap(err, "Tx update Service status & finishedAt")
		}
	}

	err = txUpdateTaskStatus(tx, task, tstate, finish, msg)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "Tx update Service status & Task status")
	}

	if !finish.IsZero() {
		svc.FinishedAt = finish
	}

	atomic.StoreInt64(&svc.Status, state)

	return nil
}

func txDeleteService(tx *sqlx.Tx, nameOrID string) error {
	_, err := tx.Exec("DELETE FROM tbl_dbaas_service WHERE id=? OR name=?", nameOrID, nameOrID)

	return err
}

const insertUserQuery = "INSERT INTO tbl_dbaas_users (id,service_id,type,username,password,role,read_only,blacklist,whitelist,created_at) VALUES (:id,:service_id,:type,:username,:password,:role,:read_only,:blacklist,:whitelist,:created_at)"

// User is for DB and Proxy
type User struct {
	ReadOnly  bool   `db:"read_only" json:"read_only"`
	ID        string `db:"id"`
	ServiceID string `db:"service_id" json:"service_id"`
	Type      string `db:"type"`
	Username  string `db:"username"`
	Password  string `db:"password"`
	Role      string `db:"role"`

	Blacklist []string `db:"-"`
	Whitelist []string `db:"-"`
	White     string   `db:"whitelist" json:"-"`
	Black     string   `db:"blacklist" json:"-"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (u User) tableName() string {
	return "tbl_dbaas_users"
}

// ListUsersByService returns []User select by serviceID and User type if assigned
func ListUsersByService(service, _type string) ([]User, error) {
	db, err := getDB(true)
	if err != nil {
		return nil, err
	}

	var users []User
	if _type == "" {
		err = db.Select(&users, "SELECT * FROM tbl_dbaas_users WHERE service_id=?", service)
	} else {
		err = db.Select(&users, "SELECT * FROM tbl_dbaas_users WHERE service_id=? AND type=?", service, _type)
	}
	if err != nil {
		return nil, errors.Wrap(err, "list []User by serviceID")
	}

	for i := range users {
		err = users[i].jsonDecode()
		if err != nil {
			return nil, err
		}
	}

	return users, nil
}

func (u *User) jsonDecode() error {
	u.Blacklist = []string{}
	u.Whitelist = []string{}

	buffer := bytes.NewBufferString(u.Black)
	if len(u.Black) > 0 {
		err := json.NewDecoder(buffer).Decode(&u.Blacklist)
		if err != nil {
			return errors.Wrap(err, "JSON decode blacklist")
		}
	}

	if len(u.White) > 0 {
		buffer.Reset()
		buffer.WriteString(u.White)

		err := json.NewDecoder(buffer).Decode(&u.Whitelist)
		if err != nil {
			return errors.Wrap(err, "JSON decode whitelist")
		}
	}

	return nil
}

func (u *User) jsonEncode() error {
	buffer := bytes.NewBuffer(nil)

	if len(u.Blacklist) > 0 {

		err := json.NewEncoder(buffer).Encode(u.Blacklist)
		if err != nil {
			return errors.Wrap(err, "JSON Encode User.Blacklist")
		}

		u.Black = buffer.String()
	}

	buffer.Reset()

	if len(u.Whitelist) > 0 {

		err := json.NewEncoder(buffer).Encode(u.Whitelist)
		if err != nil {
			return errors.Wrap(err, "JSON Encode User.Whitelist")
		}

		u.White = buffer.String()
	}

	return nil
}

// TxUpdateUsers update []User in Tx,
// If User is exist,exec update,if not exec insert.
func TxUpdateUsers(addition, update []User) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(addition) > 0 {
		err = txInsertUsers(tx, addition)
		if err != nil {
			return err
		}
	}

	if len(update) == 0 {
		err = tx.Commit()

		return errors.Wrap(err, "Tx update []User")
	}

	const query = "UPDATE tbl_dbaas_users SET type=:type,password=:password,role=:role,read_only=:read_only,blacklist=:blacklist,whitelist=:whitelist WHERE id=:id OR username=:username"
	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return errors.Wrap(err, "Tx prepare update User")
	}

	for i := range update {
		if err = update[i].jsonEncode(); err != nil {
			stmt.Close()

			return err
		}

		_, err = stmt.Exec(update[i])
		if err != nil {
			stmt.Close()

			return err
		}
	}

	stmt.Close()

	err = tx.Commit()

	return errors.Wrap(err, "Tx update []User")
}

// TxInsertUsers insert []User in Tx
func TxInsertUsers(users []User) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txInsertUsers(tx, users)
	if err != nil {
		return err
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx insert []User")
}

func txInsertUsers(tx *sqlx.Tx, users []User) error {
	stmt, err := tx.PrepareNamed(insertUserQuery)
	if err != nil {
		return errors.Wrap(err, "Tx prepare insert []User")
	}

	for i := range users {
		if len(users[i].ID) == 0 {
			continue
		}

		if err = users[i].jsonEncode(); err != nil {
			stmt.Close()

			return err
		}

		_, err = stmt.Exec(&users[i])
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx insert []User")
		}
	}

	stmt.Close()

	return errors.Wrap(err, "Tx insert []User")
}

func txDeleteUsers(tx *sqlx.Tx, id string) error {
	_, err := tx.Exec("DELETE FROM tbl_dbaas_users WHERE id=? OR service_id=?", id, id)

	return errors.Wrap(err, "Tx delete User by ID or ServiceID")
}

// TxDeleteUsers delete []User in Tx
func TxDeleteUsers(users []User) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Preparex("DELETE FROM tbl_dbaas_users WHERE id=?")
	if err != nil {
		return errors.Wrap(err, "Tx prepare delete []User")
	}

	for i := range users {
		_, err = stmt.Exec(users[i].ID)
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx delete User by ID:"+users[i].ID)
		}
	}

	stmt.Close()

	err = tx.Commit()

	return errors.Wrap(err, "Tx delete []User")
}

// DeteleServiceRelation delelte related record about Service,
// include Service,Unit,BackupStrategy,IP,Port,LocalVolume,UnitConfig.
// delete in a Tx
func DeteleServiceRelation(serviceID string, rmVolumes bool) error {
	units, err := ListUnitByServiceID(serviceID)
	if err != nil {
		return err
	}

	// recycle networking & ports & volumes
	ips := make([]IP, 0, 20)
	ports := make([]Port, 0, 20)
	volumes := make([]LocalVolume, 0, 20)

	for i := range units {

		ipl, err := ListIPByUnitID(units[i].ID)
		if err == nil {
			ips = append(ips, ipl...)
		}

		pl, err := ListPortsByUnit(units[i].ID)
		if err == nil {
			ports = append(ports, pl...)
		}

		vl, err := ListVolumesByUnitID(units[i].ID)
		if err == nil {
			volumes = append(volumes, vl...)
		}

	}

	for i := range ips {
		ips[i].Allocated = false
		ips[i].UnitID = ""
	}

	for i := range ports {
		ports[i].Allocated = false
		ports[i].Name = ""
		ports[i].UnitID = ""
		ports[i].UnitName = ""
		ports[i].Proto = ""
	}

	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = TxUpdateIPs(tx, ips)
	if err != nil {
		return err
	}

	err = TxUpdatePorts(tx, ports)
	if err != nil {
		return err
	}

	for i := range units {
		if rmVolumes {
			err = TxDeleteVolume(tx, units[i].ID)
			if err != nil {
				return err
			}
		}

		err = txDeleteUnitConfigByUnit(tx, units[i].ID)
		if err != nil {
			return err
		}
	}

	err = txDeleteBackupStrategy(tx, serviceID)
	if err != nil {
		return err
	}

	err = txDeleteUsers(tx, serviceID)
	if err != nil {
		return err
	}

	err = TxDeleteUnit(tx, serviceID)
	if err != nil {
		return err
	}

	err = txDeleteService(tx, serviceID)
	if err != nil {
		return err
	}

	err = tx.Commit()

	return errors.Wrap(err, "Detele Service relation")
}

// TxUpdateMigrateUnit update Unit and delete old LocalVolumes in a Tx
func TxUpdateMigrateUnit(u Unit, lvs []LocalVolume, reserveSAN bool) error {
	// update database :tb_unit
	// delete old localVolumes
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i := range lvs {
		if reserveSAN && strings.HasSuffix(lvs[i].VGName, "_SAN_VG") {
			continue
		}

		err := TxDeleteVolume(tx, lvs[i].ID)
		if err != nil {
			return err
		}
	}

	err = txUpdateUnit(tx, u)
	if err != nil {
		return err
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx update unit & delete volumes")
}
