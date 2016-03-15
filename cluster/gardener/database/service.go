package database

import (
	"time"

	"github.com/jmoiron/sqlx"
)

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
	volumes        []volume  `db:"-"`
	Filesystem     string    `db:"filesystem"`
	Env            string    `db:"env"`
	Cmd            string    `db:"cmd"`
	Labels         string    `db:labels`
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

type Unit struct {
	Name          string    `db:"name"`
	SoftwareID    string    `db:"software_id"`
	ServiceID     string    `db:"service_id"`
	ContainerID   string    `db:"container_id"`
	ConfigID      string    `db:"config_id"`
	StartupExecID string    `db:"startup_exec_id"`
	StartupCmd    string    `db:"startup_cmd"`
	BackupExecID  string    `db:"backup_exec_id"`
	BackupCmd     string    `db:"backup_cmd"`
	Status        uint32    `db:"status"`
	CheckInterval int       `db:"check_interval"`
	CreatedAt     time.Time `db:"created_at`
}

func (u Unit) TableName() string {
	return "tb_unit"
}

func TxInsertMultiUnit(tx *sqlx.Tx, units []*Unit) error {
	query := "INSERT INTO tb_unit (name,software_id,service_id,container_id,config_id,startup_exec_id,startup_cmd,backup_exec_id,backup_cmd,status,check_interval,create_at) VALUES (:name,:software_id,:service_id,:container_id,:config_id,:startup_exec_id,:startup_cmd,:backup_exec_id,:backup_cmd,:status,:check_interval,:create_at)"

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

type volume struct {
	Type     string // data or logs
	VolumeID string // tb_volume
}

type UnitConfig struct {
	ID         string `db:"id"`
	SoftwareID string `db:"software_id"`
	Version    int    `db:"version"`
	ParentID   string `db:"parent_id"`
	Content    string `db:"content"`
}

func (u UnitConfig) TableName() string {
	return "tb_unit_config"
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
	BackupSpaceByte  int64     `db:"backup_space"`
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
	err = db.QueryRowx("SELECT * FROM tb_service WHERE id=?", id).StructScan(&s)

	return s, err
}

func (svc *Service) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	// insert into database
	query := "INSERT INTO tb_service (id,name,description,architecture,auto_healing,auto_scaling,high_available,status,backup_space,backup_strategy_id,created_at,finished_at) VALUES (:id,:name,:description,:architecture,:auto_healing,:auto_scaling,:high_available,:status,:backup_space,:backup_strategy_id,:created_at,:finished_at)"
	_, err = db.Exec(query, svc)

	return err
}

func (svc *Service) SetServiceStatus(state int, finish time.Time) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	if finish.IsZero() {
		_, err = db.Exec("UPDATE tb_service SET status=? WHERE id=?", state, svc.ID)
		if err != nil {
			return err
		}
		// atomic.StoreInt64(&svc.Status, int64(state))
	}

	_, err = db.Exec("UPDATE tb_service SET status=?,finished_at=? WHERE id=?", state, finish, svc.ID)
	if err != nil {
		return err
	}

	// atomic.StoreInt64(&svc.Status, int64(state))
	svc.FinishedAt = finish

	return nil
}

func (svc *Service) TxSetServiceStatus(tx *sqlx.Tx, state int64, finish time.Time) error {

	if finish.IsZero() {
		_, err := tx.Exec("UPDATE tb_service SET status=? WHERE id=?", state, svc.ID)
		if err != nil {
			return err
		}
		// atomic.StoreInt64(&svc.Status, state)
	}

	_, err := tx.Exec("UPDATE tb_service SET status=?,finished_at=? WHERE id=?", state, finish, svc.ID)
	if err != nil {
		return err
	}

	// atomic.StoreInt64(&svc.Status, state)
	svc.FinishedAt = finish

	return nil
}

type User struct {
	ID        string    `db:"id"`
	Type      string    `db:"type"`
	Username  string    `db:"username"`
	Password  string    `db:"password"`
	Role      string    `db:"role"`
	CreatedAt time.Time `db:"created_at"`
}

func (u User) TableName() string {
	return "tb_users"
}
