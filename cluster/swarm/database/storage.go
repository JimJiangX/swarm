package database

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertLUNQuery = "INSERT INTO tb_lun (id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at) VALUES (:id,:name,:vg_name,:raid_group_id,:storage_system_id,:mapping_hostname,:size,:host_lun_id,:storage_lun_id,:created_at)"

type LUN struct {
	ID              string    `db:"id"`
	Name            string    `db:"name"`
	VGName          string    `db:"vg_name"`
	RaidGroupID     string    `db:"raid_group_id"`
	StorageSystemID string    `db:"storage_system_id"`
	Mappingto       string    `db:"mapping_hostname"`
	SizeByte        int       `db:"size"`
	HostLunID       int       `db:"host_lun_id"`
	StorageLunID    int       `db:"storage_lun_id"`
	CreatedAt       time.Time `db:"created_at"`
}

func (l LUN) TableName() string {
	return "tb_lun"
}

func TxInsertLUNAndVolume(lun LUN, lv LocalVolume) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.NamedExec(insertLUNQuery, &lun)
	if err != nil {
		return err
	}

	_, err = tx.NamedExec(insertLocalVolumeQuery, &lv)
	if err != nil {
		return err
	}

	return tx.Commit()
}

var DelLunMapping = LunMapping

func LunMapping(lun, host, vgName string, hlun int) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_lun SET vg_name=?,mapping_hostname=?,host_lun_id=? WHERE id=?"

	_, err = db.Exec(query, vgName, host, hlun, lun)

	return err
}

func GetLUNByID(id string) (LUN, error) {
	db, err := GetDB(true)
	if err != nil {
		return LUN{}, err
	}

	lun := LUN{}
	query := "SELECT * FROM tb_lun WHERE id=? LIMIT 1"

	err = db.Get(&lun, query, id)

	return lun, err
}

func ListLUNByName(name string) ([]LUN, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	list := make([]LUN, 0, 4)
	query := "SELECT * FROM tb_lun WHERE name=?"

	err = db.Select(&list, query, name)
	if err != nil {
		return nil, err
	}

	return list, err
}

func ListLUNByVgName(name string) ([]LUN, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	list := make([]LUN, 0, 4)
	query := "SELECT * FROM tb_lun WHERE vg_name=?"

	err = db.Select(&list, query, name)
	if err != nil {
		return nil, err
	}

	return list, err
}

func GetLUNByLunID(systemID string, id int) (LUN, error) {
	db, err := GetDB(true)
	if err != nil {
		return LUN{}, err
	}

	lun := LUN{}
	query := "SELECT * FROM tb_lun WHERE storage_system_id=? AND storage_lun_id=? LIMIT 1"

	err = db.Get(&lun, query, systemID, id)

	return lun, err
}

func CountLUNByRaidGroupID(rg string) (int, error) {
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}
	count := 0
	err = db.Get(&count, "SELECT COUNT(*) from tb_lun WHERE raid_group_id=?", rg)

	return count, err
}

func DelLUN(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_lun WHERE id=?", id)

	return err
}

func TxReleaseLun(name string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM tb_lun WHERE name OR vg_name=?", name, name)
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM tb_volumes WHERE name OR VGname=?", name, name)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func SelectHostLunIDByMapping(host string) ([]int, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var out []int
	query := "SELECT host_lun_id FROM tb_lun WHERE mapping_hostname=?"

	err = db.Select(&out, query, host)
	if err != nil {
		return nil, err
	}

	return out, err
}

func SelectLunIDBySystemID(id string) ([]int, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var out []int
	query := "SELECT storage_lun_id FROM tb_lun WHERE storage_system_id=?"

	err = db.Select(&out, query, id)
	if err != nil {
		return nil, err
	}

	return out, err
}

const insertRaidGroupQuery = "INSERT INTO tb_raid_group (id,storage_system_id,storage_rg_id,enabled) VALUES (:id,:storage_system_id,:storage_rg_id,:enabled)"

type RaidGroup struct {
	ID          string `db:"id"`
	StorageID   string `db:"storage_system_id"`
	StorageRGID int    `db:"storage_rg_id"`
	Enabled     bool   `db:"enabled"`
}

func (r RaidGroup) TableName() string {
	return "tb_raid_group"
}

func (rg RaidGroup) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertRaidGroupQuery, &rg)

	return err
}

func UpdateRaidGroupStatus(ssid string, rgid int, state bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_raid_group SET enabled=? WHERE storage_system_id=? AND storage_rg_id=?"

	_, err = db.Exec(query, state, ssid, rgid)

	return err
}

func UpdateRaidGroupStatusByID(id string, state bool) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_raid_group SET enabled=? WHERE id=?"

	_, err = db.Exec(query, state, id)

	return err
}

func SelectRaidGroupByStorageID(id string) ([]RaidGroup, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var out []RaidGroup
	query := "SELECT * FROM tb_raid_group WHERE storage_system_id=?"

	err = db.Select(&out, query, id)

	return out, err
}

func GetRaidGroup(id string, rg int) (RaidGroup, error) {
	db, err := GetDB(true)
	if err != nil {
		return RaidGroup{}, err
	}

	out := RaidGroup{}
	query := "SELECT * FROM tb_raid_group WHERE storage_system_id=? AND storage_rg_id=? LIMIT 1"

	err = db.Get(&out, query, id, rg)

	return out, err
}

func DeleteRaidGroup(id string, rg int) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_raid_group WHERE storage_system_id=? AND storage_rg_id=?", id, rg)

	return err
}

const insertHitachiStorageQuery = "INSERT INTO tb_storage_HITACHI (id,vendor,admin_unit,lun_start,lun_end,hlu_start,hlu_end) VALUES (:id,:vendor,:admin_unit,:lun_start,:lun_end,:hlu_start,:hlu_end)"

type HitachiStorage struct {
	ID        string `db:"id"`
	Vendor    string `db:"vendor"`
	AdminUnit string `db:"admin_unit"`
	LunStart  int    `db:"lun_start"`
	LunEnd    int    `db:"lun_end"`
	HluStart  int    `db:"hlu_start"`
	HluEnd    int    `db:"hlu_end"`
}

func (hds HitachiStorage) TableName() string {
	return "tb_storage_HITACHI"
}

func (hs HitachiStorage) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHitachiStorageQuery, &hs)

	return err
}

const insertHuaweiStorageQuery = "INSERT INTO tb_storage_HUAWEI (id,vendor,ip_addr,username,password,hlu_start,hlu_end) VALUES (:id,:vendor,:ip_addr,:username,:password,:hlu_start,:hlu_end)"

type HuaweiStorage struct {
	ID       string `db:"id"`
	Vendor   string `db:"vendor"`
	IPAddr   string `db:"ip_addr"`
	Username string `db:"username"`
	Password string `db:"password"`
	HluStart int    `db:"hlu_start"`
	HluEnd   int    `db:"hlu_end"`
}

func (h HuaweiStorage) TableName() string {
	return "tb_storage_HUAWEI"
}

func (hs HuaweiStorage) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHuaweiStorageQuery, &hs)

	return err
}

const insertLocalVolumeQuery = "INSERT INTO tb_volumes (id,name,unit_id,size,VGname,driver,fstype) VALUES (:id,:name,:unit_id,:size,:VGname,:driver,:fstype)"

type LocalVolume struct {
	Size       int    `db:"size"`
	ID         string `db:"id"`
	Name       string `db:"name"`
	UnitID     string `db:"unit_id"`
	VGName     string `db:"VGname"`
	Driver     string `db:"driver"`
	Filesystem string `db:"fstype"`
}

func (LocalVolume) TableName() string {
	return "tb_volumes"
}

func InsertLocalVolume(lv LocalVolume) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertLocalVolumeQuery, &lv)

	return err
}

func UpdateLocalVolume(nameOrID string, size int) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE tb_volumes SET size=? WHERE id=? OR name=?", size, nameOrID, nameOrID)

	return err
}

func TxUpdateLocalVolume(tx *sqlx.Tx, nameOrID string, size int) error {
	_, err := tx.Exec("UPDATE tb_volumes SET size=? WHERE id=? OR name=?", size, nameOrID, nameOrID)

	return err
}

func DeleteLocalVoume(nameOrID string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_volumes WHERE id=? OR name=?", nameOrID, nameOrID)

	return err
}

func TxDeleteVolume(tx *sqlx.Tx, nameOrID string) error {
	_, err := tx.Exec("DELETE FROM tb_volumes WHERE id=? OR name=? OR unit_id=?", nameOrID, nameOrID, nameOrID)

	return err
}

func TxDeleteVolumes(volumes []LocalVolume) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	stmt, err := tx.Preparex("DELETE FROM tb_volumes WHERE id=?")
	if err != nil {
		return err
	}

	defer stmt.Close()

	for i := range volumes {
		_, err = stmt.Exec(volumes[i].ID)
		if err != nil {
			return errors.Wrap(err, "delete volumes:"+volumes[i].ID)
		}
	}

	return nil
}

func GetLocalVolume(nameOrID string) (LocalVolume, error) {
	lv := LocalVolume{}

	db, err := GetDB(true)
	if err != nil {
		return lv, err
	}

	query := "SELECT * FROM tb_volumes WHERE id=? OR name=?"

	err = db.Get(&lv, query, nameOrID, nameOrID)

	return lv, err
}

func SelectVolumeByVG(name string) ([]LocalVolume, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var lvs []LocalVolume
	query := "SELECT * FROM tb_volumes WHERE VGname=?"
	err = db.Select(&lvs, query, name)
	if err != nil {
		return nil, err
	}

	return lvs, nil
}

func SelectVolumesByUnitID(id string) ([]LocalVolume, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var lvs []LocalVolume
	query := "SELECT * FROM tb_volumes WHERE unit_id=?"
	err = db.Select(&lvs, query, id)
	if err != nil {
		return nil, err
	}

	return lvs, nil
}

func GetStorageByID(id string) (*HitachiStorage, *HuaweiStorage, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, nil, err
	}

	hitachi, huawei := &HitachiStorage{}, &HuaweiStorage{}

	err = db.Get(hitachi, "SELECT * FROM tb_storage_HITACHI WHERE id=?", id)
	if err == nil {
		return hitachi, nil, nil
	}

	err = db.Get(huawei, "SELECT * FROM tb_storage_HUAWEI WHERE id=?", id)
	if err == nil {
		return nil, huawei, nil
	}

	return nil, nil, err
}

func ListStorageID() ([]string, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var hitachi []string
	err = db.Select(&hitachi, "SELECT id FROM tb_storage_HITACHI")
	if err != nil {
		return nil, err
	}

	var huawei []string
	err = db.Select(&huawei, "SELECT id FROM tb_storage_HUAWEI")
	if err != nil {
		return nil, err
	}

	out := make([]string, len(hitachi)+len(huawei))

	length := copy(out, hitachi)
	copy(out[length:], huawei)

	return out, nil
}

func DeleteStorageByID(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	r, err := db.Exec("DELETE FROM tb_storage_HITACHI WHERE id=?", id)
	if err == nil {
		num, err := r.RowsAffected()
		if num > 0 && err == nil {
			return nil
		}
	}

	r, err = db.Exec("DELETE FROM tb_storage_HUAWEI WHERE id=?", id)
	if err == nil {
		num, err := r.RowsAffected()
		if num > 0 && err == nil {
			return nil
		}
	}

	return err
}
