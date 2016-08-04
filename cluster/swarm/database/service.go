package database

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func (c Container) TableName() string {
	return "tb_container"
}

func TxInsertMultiContainer(tx *sqlx.Tx, clist []*Container) error {
	query := "INSERT INTO tb_container (id,node_id,networking_id,ip_addr,ports,image,cpu_set,cpu_shares,ncpu,mem,mem_swap,storage_type,network_mode,volume_driver,volumes_from,filesystem,env,cmd,labels,create_at) VALUES (:id,:node_id,:networking_id,:ip_addr,:ports,:image,:cpu_set,:cpu_shares,:ncpu,:mem,:mem_swap,:storage_type,:network_mode,:volume_driver,:volumes_from,:filesystem,:env,:cmd,:labels,:create_at)"

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

const insertUnitQuery = "INSERT INTO tb_unit (id,name,type,image_id,image_name,service_id,node_id,container_id,unit_config_id,network_mode,status,latest_error,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:node_id,:container_id,:unit_config_id,:network_mode,:status,:latest_error,:check_interval,:created_at)"

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

func (u Unit) TableName() string {
	return "tb_unit"
}

func GetUnit(nameOrID string) (Unit, error) {
	u := Unit{}

	db, err := GetDB(true)
	if err != nil {
		return u, err
	}

	err = db.Get(&u, "SELECT * FROM tb_unit WHERE id=? OR name=? OR container_id=?", nameOrID, nameOrID, nameOrID)

	return u, err
}

func TxInsertUnit(tx *sqlx.Tx, unit Unit) error {
	_, err := tx.NamedExec(insertUnitQuery, &unit)

	return err
}

func TxInsertMultiUnit(tx *sqlx.Tx, units []*Unit) error {
	stmt, err := tx.PrepareNamed(insertUnitQuery)
	if err != nil {
		return err
	}

	for i := range units {
		if units[i] == nil {
			continue
		}

		_, err = stmt.Exec(units[i])
		if err != nil {
			stmt.Close()

			return err
		}
	}

	return stmt.Close()
}

func UpdateUnitInfo(unit Unit) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_unit SET name=:name,type=:type,image_id=:image_id,image_name=:image_name,service_id=:service_id,node_id=:node_id,container_id=:container_id,unit_config_id=:unit_config_id,network_mode=:network_mode,status=:status,latest_error=:latest_error,check_interval=:check_interval,created_at=:created_at WHERE id=:id"

	_, err = db.NamedExec(query, &unit)

	return err
}

func TxUpdateUnit(tx *sqlx.Tx, unit Unit) error {
	//	query := "UPDATE tb_unit SET name=:name,type=:type,image_id=:image_id,image_name=:image_name,service_id=:service_id,node_id=:node_id,container_id=:container_id,unit_config_id=:unit_config_id,network_mode=:network_mode,status=:status,check_interval=:check_interval,created_at=:created_at WHERE id=:id"
	query := "UPDATE tb_unit SET node_id=:node_id,container_id=:container_id,status=:status,latest_error=:latest_error,created_at=:created_at WHERE id=:id"

	_, err := tx.NamedExec(query, unit)

	return err
}

func (u *Unit) StatusCAS(operator string, old, value int64) error {
	if operator == "!=" {
		operator = "<>"
	}

	query := fmt.Sprintf("UPDATE tb_unit SET status=? WHERE id=? AND status%s?", operator)

	db, err := GetDB(false)
	if err != nil {
		return err
	}

	r, err := db.Exec(query, value, u.ID, old)
	if err != nil {
		db, err = GetDB(true)
		if err != nil {
			return err
		}

		r, err = db.Exec(query, value, u.ID, old)
		if err != nil {
			return errors.Wrap(err, "Update Unit Status")
		}
	}

	n, err := r.RowsAffected()
	if n == 1 {
		atomic.StoreInt64(&u.Status, value)

		return nil

	} else if err != nil {

		return errors.Wrap(err, "Update Unit Status Affected Error")
	}

	return errors.Errorf("Forbid To Set Unit Status")
}

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

	return tx.Commit()
}

func TxUpdateUnitStatus(unit *Unit, status int64, msg string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txUpdateUnitStatus(tx, unit, status, msg)

	return tx.Commit()
}

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

	return tx.Commit()
}

func txUpdateUnitStatus(tx *sqlx.Tx, unit *Unit, status int64, msg string) error {

	_, err := tx.Exec("UPDATE tb_unit SET status=?,latest_error=? WHERE id=?", status, msg, unit.ID)
	if err != nil {
		return errors.Wrap(err, "tx Update Unit Status")
	}

	atomic.StoreInt64(&unit.Status, status)
	unit.LatestError = msg

	return nil
}

func TxDeleteUnit(tx *sqlx.Tx, nameOrID string) error {
	_, err := tx.Exec("DELETE FROM tb_unit WHERE id=? OR name=? OR service_id=?", nameOrID, nameOrID, nameOrID)

	return err
}

func ListUnitByServiceID(ID string) ([]Unit, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	units := make([]Unit, 0, 5)
	err = db.Select(&units, "SELECT * FROM tb_unit WHERE service_id=?", ID)
	if err != nil {
		return nil, err
	}

	return units, nil
}

func CountUnitByNode(id string) (int, error) {
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}
	count := 0
	err = db.Get(&count, "SELECT COUNT(*) from tb_unit WHERE node_id=?", id)

	return count, err
}

func SaveUnitConfigToDisk(unit *Unit, config UnitConfig) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if unit != nil && unit.ID != "" {
		query := "UPDATE tb_unit SET unit_config_id=? WHERE id=?"
		_, err = tx.Exec(query, config.ID, unit.ID)
		if err != nil {
			return err
		}
	}

	config.UnitID = unit.ID

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	unit.ConfigID = config.ID

	return nil
}

const insertServiceQuery = "INSERT INTO tb_service (id,name,description,architecture,business_code,auto_healing,auto_scaling,high_available,status,backup_max_size,backup_files_retention,created_at,finished_at) VALUES (:id,:name,:description,:architecture,:business_code,:auto_healing,:auto_scaling,:high_available,:status,:backup_max_size,:backup_files_retention,:created_at,:finished_at)"

type Service struct {
	ID                string `db:"id"`
	Name              string `db:"name"`
	Description       string `db:"description"`
	Architecture      string `db:"architecture"`
	BusinessCode      string `db:"business_code"`
	AutoHealing       bool   `db:"auto_healing"`
	AutoScaling       bool   `db:"auto_scaling"`
	HighAvailable     bool   `db:"high_available"`
	Status            int64  `db:"status"`
	BackupMaxSizeByte int    `db:"backup_max_size"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	BackupFilesRetention int       `db:"backup_files_retention"`
	CreatedAt            time.Time `db:"created_at"`
	FinishedAt           time.Time `db:"finished_at"`
}

func (svc Service) TableName() string {
	return "tb_service"
}

func ListServices() ([]Service, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	services := make([]Service, 0, 10)
	err = db.Select(&services, "SELECT * FROM tb_service")
	if err != nil {
		return nil, err
	}

	return services, nil
}

func GetService(nameOrID string) (Service, error) {
	db, err := GetDB(true)
	if err != nil {
		return Service{}, err
	}

	s := Service{}
	err = db.Get(&s, "SELECT * FROM tb_service WHERE id=? OR name=?", nameOrID, nameOrID)

	return s, err
}

func UpdateServcieDescription(ID, des string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_service SET description=? WHERE id=?", des, ID)

	return err
}

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

	return tx.Commit()
}

func txInsertSerivce(tx *sqlx.Tx, svc Service) error {
	// insert into database
	_, err := tx.NamedExec(insertServiceQuery, &svc)

	return err
}

func (svc *Service) SetServiceStatus(state int64, finish time.Time) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	if finish.IsZero() {
		_, err = db.Exec("UPDATE tb_service SET status=? WHERE id=?", state, svc.ID)
		if err != nil {
			return err
		}

		atomic.StoreInt64(&svc.Status, state)

		return nil
	}

	_, err = db.Exec("UPDATE tb_service SET status=?,finished_at=? WHERE id=?", state, finish, svc.ID)
	if err != nil {
		return err
	}

	atomic.StoreInt64(&svc.Status, state)
	svc.FinishedAt = finish

	return nil
}

func TxSetServiceStatus(svc *Service, task *Task, state, tstate int64, finish time.Time, msg string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if finish.IsZero() {
		_, err := tx.Exec("UPDATE tb_service SET status=? WHERE id=?", state, svc.ID)
		if err != nil {
			return err
		}
	} else {
		_, err := tx.Exec("UPDATE tb_service SET status=?,finished_at=? WHERE id=?", state, finish, svc.ID)
		if err != nil {
			return err
		}
	}

	err = txUpdateTaskStatus(tx, task, tstate, finish, msg)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	if !finish.IsZero() {
		svc.FinishedAt = finish
	}

	atomic.StoreInt64(&svc.Status, state)

	return nil
}

func txDeleteService(tx *sqlx.Tx, nameOrID string) error {
	_, err := tx.Exec("DELETE FROM tb_service WHERE id=? OR name=?", nameOrID, nameOrID)

	return err
}

const insertUserQuery = "INSERT INTO tb_users (id,service_id,type,username,password,role,permission,blacklist,whitelist,created_at) VALUES (:id,:service_id,:type,:username,:password,:role,:permission,:blacklist,:whitelist,:created_at)"

type User struct {
	ID         string   `db:"id"`
	ServiceID  string   `db:"service_id"`
	Type       string   `db:"type"`
	Username   string   `db:"username"`
	Password   string   `db:"password"`
	Role       string   `db:"role"`
	Permission string   `db:"permission"`
	Blacklist  []string `db:"-"`
	Whitelist  []string `db:"-"`
	White      string   `db:"whitelist" json:"-"`
	Black      string   `db:"blacklist" json:"-"`

	CreatedAt time.Time `db:"created_at"`
}

func (u User) TableName() string {
	return "tb_users"
}

func ListUsersByService(service, _type string) ([]User, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var users []User
	if _type == "" {
		err = db.Select(&users, "SELECT * FROM tb_users WHERE service_id=?", service)
	} else {
		err = db.Select(&users, "SELECT * FROM tb_users WHERE service_id=? AND type=?", service, _type)
	}
	if err != nil {
		return nil, err
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
			return err
		}
	}

	if len(u.White) > 0 {
		buffer.Reset()
		buffer.WriteString(u.White)

		err := json.NewDecoder(buffer).Decode(&u.Whitelist)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u *User) jsonEncode() error {
	buffer := bytes.NewBuffer(nil)
	if len(u.Blacklist) > 0 {
		err := json.NewEncoder(buffer).Encode(u.Blacklist)
		if err != nil {
			return err
		}
		u.Black = buffer.String()
	}

	buffer.Reset()

	if len(u.Whitelist) > 0 {
		err := json.NewEncoder(buffer).Encode(u.Whitelist)
		if err != nil {
			return err
		}
		u.White = buffer.String()
	}

	return nil
}

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
		return tx.Commit()
	}

	query := "UPDATE tb_users SET type=:type,password=:password,role=:role,permission=:permission,blacklist=:blacklist,whitelist=:whitelist WHERE id=:id OR username=:username"
	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
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

	return tx.Commit()
}

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

	return tx.Commit()
}

func txInsertUsers(tx *sqlx.Tx, users []User) error {
	stmt, err := tx.PrepareNamed(insertUserQuery)
	if err != nil {
		return err
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

			return err
		}
	}

	return stmt.Close()
}

func txDeleteUsers(tx *sqlx.Tx, id string) error {
	_, err := tx.Exec("DELETE FROM tb_users WHERE id=? OR service_id=?", id, id)

	return err
}

func TxDeleteUsers(users []User) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Preparex("DELETE FROM tb_users WHERE id=?")
	if err != nil {
		return err
	}

	for i := range users {
		_, err = stmt.Exec(users[i].ID)
		if err != nil {
			stmt.Close()

			return err
		}
	}

	stmt.Close()

	return tx.Commit()
}

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

		vl, err := SelectVolumesByUnitID(units[i].ID)
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

	err = TxUpdateMultiIPValue(tx, ips)
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

	return tx.Commit()
}
