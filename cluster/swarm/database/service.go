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
	NodeID      string `db:"node_id"`
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

func TxInsertUnit(tx *sqlx.Tx, unit *Unit) error {
	query := "INSERT INTO tb_unit (id,name,type,image_id,image_name,service_id,node_id,container_id,unit_config_id,network_mode,status,check_interval,created_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:node_id,:container_id,:unit_config_id,:network_mode,:status,:check_interval,:created_at)"
	_, err := tx.NamedExec(query, unit)

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

func TxDelUnit(tx *sqlx.Tx, id string) error {

	_, err := tx.Exec("DELETE FROM tb_unit WHERE id=?", id)

	return err
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
	ID               string    `db:"id"`
	Name             string    `db:"name"`
	Description      string    `db:"description"`
	Architecture     string    `db:"architecture"`
	AutoHealing      bool      `db:"auto_healing"`
	AutoScaling      bool      `db:"auto_scaling"`
	HighAvailable    bool      `db:"high_available"`
	Status           int64     `db:"status"`
	BackupSpaceByte  int       `db:"backup_space"`
	BackupStrategyID string    `db:"backup_strategy_id"`
	CreatedAt        time.Time `db:"created_at"`
	FinishedAt       time.Time `db:"finished_at"`
}

func (svc Service) TableName() string {
	return "tb_service"
}

func GetService(id string) (Service, error) {
	db, err := GetDB(true)
	if err != nil {
		return Service{}, err
	}

	s := Service{}
	err = db.Get(&s, "SELECT * FROM tb_service WHERE id=?", id)

	return s, err
}

func TxSaveService(svc *Service, strategy *BackupStrategy, task *Task, users []User) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txInsertSerivce(tx, svc)
	if err != nil {
		return err
	}

	err = TxInsertTask(tx, task)
	if err != nil {
		return err
	}

	err = TxInsertBackupStrategy(tx, strategy)
	if err != nil {
		return err
	}

	err = TxInsertMultipleUsers(tx, users)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func txInsertSerivce(tx *sqlx.Tx, svc *Service) error {
	// insert into database
	query := "INSERT INTO tb_service (id,name,description,architecture,auto_healing,auto_scaling,high_available,status,backup_space,backup_strategy_id,created_at,finished_at) VALUES (:id,:name,:description,:architecture,:auto_healing,:auto_scaling,:high_available,:status,:backup_space,:backup_strategy_id,:created_at,:finished_at)"
	_, err := tx.NamedExec(query, svc)

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

type User struct {
	ID        string    `db:"id"`
	ServiceID string    `db:"service_id"`
	Type      string    `db:"type"`
	Username  string    `db:"username"`
	Password  string    `db:"password"`
	Role      string    `db:"role"`
	CreatedAt time.Time `db:"created_at"`
}

func (u User) TableName() string {
	return "tb_users"
}

func TxInsertMultipleUsers(tx *sqlx.Tx, users []User) error {
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
