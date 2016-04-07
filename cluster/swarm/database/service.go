package database

import (
	"encoding/json"
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
	CreatedAt     time.Time `db:"created_at`
}

func (u Unit) TableName() string {
	return "tb_unit"
}

func TxInsertMultiUnit(tx *sqlx.Tx, units []*Unit) error {
	query := "INSERT INTO tb_unit (id,name,type,image_id,image_name,service_id,node_id,container_id,unit_config_id,network_mode,status,check_interval,create_at) VALUES (:id,:name,:type,:image_id,:image_name,:service_id,:node_id,:container_id,:unit_config_id,:network_mode,:status,:check_interval,:create_at)"

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

type UnitConfig struct {
	ID            string          `db:"id"`
	ImageID       string          `db:"image_id"`
	Path          string          `db:"config_file_path"`
	Version       int             `db:"version"`
	ParentID      string          `db:"parent_id"`
	Content       string          `db:"content"`         // map[string]interface{}
	ConfigKeySets string          `db:"config_key_sets"` // map[string]bool
	KeySets       map[string]bool `db:"-"`

	CreateAt time.Time `db:"create_at"`
}

func (u UnitConfig) TableName() string {
	return "tb_unit_config"
}

func (c *UnitConfig) encode() error {
	if len(c.KeySets) > 0 {
		data, err := json.Marshal(c.KeySets)
		if err != nil {
			return err
		}

		c.ConfigKeySets = string(data)
	}

	return nil
}

func (c *UnitConfig) decode() error {
	if len(c.ConfigKeySets) > 0 {
		err := json.Unmarshal([]byte(c.ConfigKeySets), &c.KeySets)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetUnitConfigByID(id string) (*UnitConfig, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	config := &UnitConfig{}
	query := "SELECT * FROM tb_unit_config WHERE id=? OR image_id=?"

	err = db.QueryRowx(query, id, id).StructScan(config)
	if err != nil {
		return nil, err
	}

	err = config.decode()

	return config, err
}

func SaveUnitConfigToDisk(unit *Unit, config UnitConfig) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if unit != nil && unit.ID != "" && unit.ConfigID != "" {
		query := "UPDATE tb_unit SET config_id=? WHERE id=?"
		_, err = tx.Exec(query, unit.ConfigID, unit.ID)
		if err != nil {
			return err
		}
	}

	err = TXInsertUnitConfig(tx, &config)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func TXInsertUnitConfig(tx *sqlx.Tx, config *UnitConfig) error {
	err := config.encode()
	if err != nil {
		return err
	}

	query := "INSERT INTO tb_unit_config (id,image_id,config_file_path,version,parent_id,content,create_at) VALUES (:id,:image_id,:config_file_path,:version,:parent_id,:content,:create_at)"

	_, err = tx.NamedExec(query, config)

	return err
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
	_, err = db.NamedExec(query, svc)

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

func (svc *Service) TxSetServiceStatus(tx *sqlx.Tx, state int64, finish time.Time) error {

	if finish.IsZero() {
		_, err := tx.Exec("UPDATE tb_service SET status=? WHERE id=?", state, svc.ID)
		if err != nil {
			return err
		}

		atomic.StoreInt64(&svc.Status, state)

		return nil
	}

	_, err := tx.Exec("UPDATE tb_service SET status=?,finished_at=? WHERE id=?", state, finish, svc.ID)
	if err != nil {
		return err
	}

	atomic.StoreInt64(&svc.Status, state)
	svc.FinishedAt = finish

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
