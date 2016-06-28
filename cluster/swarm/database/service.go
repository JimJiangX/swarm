package database

import (
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
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

type Unit struct {
	ID          string `db:"id"`
	Name        string `db:"name"` // <unit_id_8bit>_<service_name>
	Type        string `db:"type"` // switch_manager/upproxy/upsql
	ImageID     string `db:"image_id"`
	ImageName   string `db:"image_name"` //<image_name>_<image_version>
	ServiceID   string `db:"service_id"`
	EngineID    string `db:"node_id"` // engine.ID
	ContainerID string `db:"container_id"`
	ConfigID    string `db:"unit_config_id"`
	NetworkMode string `db:"network_mode"`

	Status        uint32    `db:"status"`
	CheckInterval int       `db:"check_interval"`
	CreatedAt     time.Time `db:"created_at"`
}

func (u Unit) TableName() string {
	return "tb_unit"
}

func GetUnit(NameOrID string) (Unit, error) {
	u := Unit{}

	db, err := GetDB(true)
	if err != nil {
		return u, err
	}

	err = db.Get(&u, "SELECT * FROM tb_unit WHERE id=? OR name=? OR container_id=?", NameOrID, NameOrID, NameOrID)

	return u, err
}

func TxInsertUnit(tx *sqlx.Tx, unit Unit) error {
	query := "INSERT INTO tb_unit (id,name,type,image_id,image_name,service_id,node_id,container_id,unit_config_id,network_mode,status,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:node_id,:container_id,:unit_config_id,:network_mode,:status,:check_interval,:created_at)"
	_, err := tx.NamedExec(query, &unit)

	return err
}

func TxInsertMultiUnit(tx *sqlx.Tx, units []*Unit) error {
	query := "INSERT INTO tb_unit (id,name,type,image_id,image_name,service_id,node_id,container_id,unit_config_id,network_mode,status,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:node_id,:container_id,:unit_config_id,:network_mode,:status,:check_interval,:created_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range units {

		if units[i] == nil {
			continue
		}

		_, err = stmt.Exec(units[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func UpdateUnitInfo(unit Unit) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_unit SET name=:name,type=:type,image_id=:image_id,image_name=:image_name,service_id=:service_id,node_id=:node_id,container_id=:container_id,unit_config_id=:unit_config_id,network_mode=:network_mode,status=:status,check_interval=:check_interval,created_at=:created_at WHERE id=:id"

	_, err = db.NamedExec(query, &unit)

	return err
}

func TxDeleteUnit(tx *sqlx.Tx, NameOrID string) error {
	_, err := tx.Exec("DELETE FROM tb_unit WHERE id=? OR name=? OR service_id=?", NameOrID, NameOrID, NameOrID)

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

type Service struct {
	ID                string `db:"id"`
	Name              string `db:"name"`
	Description       string `db:"description"`
	Architecture      string `db:"architecture"`
	AutoHealing       bool   `db:"auto_healing"`
	AutoScaling       bool   `db:"auto_scaling"`
	HighAvailable     bool   `db:"high_available"`
	BusinessCode      string `db:"business_code"`
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

func GetService(NameOrID string) (Service, error) {
	db, err := GetDB(true)
	if err != nil {
		return Service{}, err
	}

	s := Service{}
	err = db.Get(&s, "SELECT * FROM tb_service WHERE id=? OR name=?", NameOrID, NameOrID)

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
		err = TxInsertUsers(tx, users)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func txInsertSerivce(tx *sqlx.Tx, svc Service) error {
	// insert into database
	query := "INSERT INTO tb_service (id,name,description,architecture,business_code,auto_healing,auto_scaling,high_available,status,backup_max_size,backup_files_retention,created_at,finished_at) VALUES (:id,:name,:description,:architecture,:business_code,:auto_healing,:auto_scaling,:high_available,:status,:backup_max_size,:backup_files_retention,:created_at,:finished_at)"
	_, err := tx.NamedExec(query, &svc)

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

	err = TxUpdateTaskStatus(tx, task, int(tstate), finish, msg)
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

func txDeleteService(tx *sqlx.Tx, NameOrID string) error {
	_, err := tx.Exec("DELETE FROM tb_service WHERE id=? OR name=?", NameOrID, NameOrID)

	return err
}

type User struct {
	ID         string    `db:"id"`
	ServiceID  string    `db:"service_id"`
	Type       string    `db:"type"`
	Username   string    `db:"username"`
	Password   string    `db:"password"`
	Role       string    `db:"role"`
	Permission string    `db:"permission"`
	CreatedAt  time.Time `db:"created_at"`
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

	return users, nil
}

func TxInsertUsers(tx *sqlx.Tx, users []User) error {
	query := "INSERT INTO tb_users (id,service_id,type,username,password,role,created_at) VALUES (:id,:service_id,:type,:username,:password,:role,:created_at)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}

	for i := range users {
		if users[i] == (User{}) {
			continue
		}

		_, err = stmt.Exec(&users[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func txDeleteUsers(tx *sqlx.Tx, id string) error {
	_, err := tx.Exec("DELETE FROM tb_users WHERE id=? OR service_id=?", id, id)

	return err
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
			err = TxDeleteVolumes(tx, units[i].ID)
			if err != nil {
				return err
			}
		}

		// TODO:add later when fix unitConfig delete mistake
		// err = txDeleteUnitConfig(tx, units[i].ConfigID)
		// if err != nil {
		//	 return err
		// }
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
