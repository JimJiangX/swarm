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
		return errors.Wrap(err, "Tx Insert LUN")
	}

	_, err = tx.NamedExec(insertLocalVolumeQuery, &lv)
	if err != nil {
		return errors.Wrap(err, "Tx Insert LocalVolume")
	}

	return tx.Commit()
}

var DelLunMapping = LunMapping

func LunMapping(lun, host, vgName string, hlun int) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_lun SET vg_name=?,mapping_hostname=?,host_lun_id=? WHERE id=?"

	_, err = db.Exec(query, vgName, host, hlun, lun)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, vgName, host, hlun, lun)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Update Lun Mapping")
}

func GetLUNByID(id string) (LUN, error) {
	db, err := GetDB(false)
	if err != nil {
		return LUN{}, err
	}

	lun := LUN{}
	const query = "SELECT * FROM tb_lun WHERE id=? LIMIT 1"

	err = db.Get(&lun, query, id)
	if err == nil {
		return lun, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return lun, err
	}

	err = db.Get(&lun, query, id)
	if err == nil {
		return lun, nil
	}

	return lun, errors.Wrap(err, "Get LUN By ID:"+id)
}

func ListLUNByName(name string) ([]LUN, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []LUN
	const query = "SELECT * FROM tb_lun WHERE name=?"

	err = db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "Select []LUN By Name:"+name)
}

func ListLUNByVgName(name string) ([]LUN, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var list []LUN
	const query = "SELECT * FROM tb_lun WHERE vg_name=?"

	err = db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "Select []LUN By VGName:"+name)
}

func GetLUNByLunID(systemID string, id int) (LUN, error) {
	db, err := GetDB(false)
	if err != nil {
		return LUN{}, err
	}

	lun := LUN{}
	const query = "SELECT * FROM tb_lun WHERE storage_system_id=? AND storage_lun_id=? LIMIT 1"

	err = db.Get(&lun, query, systemID, id)
	if err == nil {
		return lun, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return lun, err
	}

	err = db.Get(&lun, query, systemID, id)
	if err == nil {
		return lun, nil
	}

	return lun, errors.Wrapf(err, "Get LUN By:StorageSystemID=%s,StorageLunID=%d", systemID, id)
}

func CountLUNByRaidGroupID(rg string) (int, error) {
	db, err := GetDB(false)
	if err != nil {
		return 0, err
	}

	count := 0
	const query = "SELECT COUNT(*) from tb_lun WHERE raid_group_id=?"

	err = db.Get(&count, query, rg)
	if err == nil {
		return count, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return 0, err
	}

	err = db.Get(&count, query, rg)
	if err == nil {
		return count, nil
	}

	return 0, errors.Wrap(err, "Count LUN By RaidGroupID:"+rg)
}

func DelLUN(id string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tb_lun WHERE id=?"

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete LUN By ID:"+id)
}

func TxReleaseLun(name string) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM tb_lun WHERE name OR vg_name=?", name, name)
	if err != nil {
		return errors.Wrap(err, "Tx Delete LUN")
	}

	_, err = tx.Exec("DELETE FROM tb_volumes WHERE name OR VGname=?", name, name)
	if err != nil {
		return errors.Wrap(err, "Tx Delete LocalVolume")
	}

	return tx.Commit()
}

func SelectHostLunIDByMapping(host string) ([]int, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var out []int
	const query = "SELECT host_lun_id FROM tb_lun WHERE mapping_hostname=?"

	err = db.Select(&out, query, host)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, host)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "Select []LUN HostLunID")
}

func SelectLunIDBySystemID(id string) ([]int, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var out []int
	const query = "SELECT storage_lun_id FROM tb_lun WHERE storage_system_id=?"

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "Select []LUN StorageLunID")
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
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertRaidGroupQuery, &rg)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertRaidGroupQuery, &rg)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Insert RaidGroup")
}

func UpdateRaidGroupStatus(ssid string, rgid int, state bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_raid_group SET enabled=? WHERE storage_system_id=? AND storage_rg_id=?"

	_, err = db.Exec(query, state, ssid, rgid)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, state, ssid, rgid)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Update RaidGroup")
}

func UpdateRaidGroupStatusByID(id string, state bool) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_raid_group SET enabled=? WHERE id=?"

	_, err = db.Exec(query, state, id)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, state, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Update RaidGroup")
}

func SelectRaidGroupByStorageID(id string) ([]RaidGroup, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var out []RaidGroup
	const query = "SELECT * FROM tb_raid_group WHERE storage_system_id=?"

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "Select []RaidGroup")
}

func GetRaidGroup(id string, rg int) (RaidGroup, error) {
	db, err := GetDB(false)
	if err != nil {
		return RaidGroup{}, err
	}

	out := RaidGroup{}
	const query = "SELECT * FROM tb_raid_group WHERE storage_system_id=? AND storage_rg_id=? LIMIT 1"

	err = db.Get(&out, query, id, rg)
	if err == nil {
		return out, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return out, err
	}

	return out, errors.Wrap(err, "Get RaidGroup")
}

func DeleteRaidGroup(id string, rg int) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tb_raid_group WHERE storage_system_id=? AND storage_rg_id=?"

	_, err = db.Exec(query, id, rg)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, id, rg)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete RaidGroup")
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
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHitachiStorageQuery, &hs)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHitachiStorageQuery, &hs)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Insert HitachiStorage")
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
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHuaweiStorageQuery, &hs)
	if err != nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHuaweiStorageQuery, &hs)
	if err != nil {
		return nil
	}

	return errors.Wrap(err, "Insert HuaweiStorage")
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
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertLocalVolumeQuery, &lv)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertLocalVolumeQuery, &lv)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Insert LOcalVolume")
}

func UpdateLocalVolume(nameOrID string, size int) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tb_volumes SET size=? WHERE id=? OR name=?"

	_, err = db.Exec(query, size, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, size, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Update LocalVolume")
}

func TxUpdateLocalVolume(tx *sqlx.Tx, nameOrID string, size int) error {
	_, err := tx.Exec("UPDATE tb_volumes SET size=? WHERE id=? OR name=?", size, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx Update LocalVolume")
}

func DeleteLocalVoume(nameOrID string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tb_volumes WHERE id=? OR name=?"

	_, err = db.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	db, err = GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete LocalVolume By nameOrID:"+nameOrID)
}

func TxDeleteVolume(tx *sqlx.Tx, nameOrID string) error {
	_, err := tx.Exec("DELETE FROM tb_volumes WHERE id=? OR name=? OR unit_id=?", nameOrID, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx Delete LocalVolume")
}

func TxDeleteVolumes(volumes []LocalVolume) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	stmt, err := tx.Preparex("DELETE FROM tb_volumes WHERE id=?")
	if err != nil {
		return errors.Wrap(err, "Tx Prepare Delete []LocalVolume")
	}

	defer stmt.Close()

	for i := range volumes {
		_, err = stmt.Exec(volumes[i].ID)
		if err != nil {
			return errors.Wrap(err, "Tx Delete LocalVolume:"+volumes[i].ID)
		}
	}

	return nil
}

func GetLocalVolume(nameOrID string) (LocalVolume, error) {
	lv := LocalVolume{}

	db, err := GetDB(false)
	if err != nil {
		return lv, err
	}

	const query = "SELECT * FROM tb_volumes WHERE id=? OR name=?"

	err = db.Get(&lv, query, nameOrID, nameOrID)
	if err == nil {
		return lv, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return lv, err
	}

	err = db.Get(&lv, query, nameOrID, nameOrID)
	if err == nil {
		return lv, nil
	}

	return lv, errors.Wrap(err, "Get LocalVolume By nameOrID:"+nameOrID)
}

func SelectVolumeByVG(name string) ([]LocalVolume, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var lvs []LocalVolume
	const query = "SELECT * FROM tb_volumes WHERE VGname=?"

	err = db.Select(&lvs, query, name)
	if err == nil {
		return lvs, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&lvs, query, name)
	if err == nil {
		return lvs, nil
	}

	return nil, errors.Wrap(err, "Select []LocalVolume By VGName:"+name)
}

func SelectVolumesByUnitID(id string) ([]LocalVolume, error) {
	db, err := GetDB(false)
	if err != nil {
		return nil, err
	}

	var lvs []LocalVolume
	const query = "SELECT * FROM tb_volumes WHERE unit_id=?"

	err = db.Select(&lvs, query, id)
	if err == nil {
		return lvs, nil
	}

	db, err = GetDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&lvs, query, id)
	if err == nil {
		return lvs, nil
	}

	return nil, errors.Wrap(err, "Select []LocalVolume By UnitID:"+id)
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

	return nil, nil, errors.Wrap(err, "Not Found Storage By ID:"+id)
}

func ListStorageID() ([]string, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var hitachi []string
	err = db.Select(&hitachi, "SELECT id FROM tb_storage_HITACHI")
	if err != nil {
		return nil, errors.Wrap(err, "Select []HitachiStorage")
	}

	var huawei []string
	err = db.Select(&huawei, "SELECT id FROM tb_storage_HUAWEI")
	if err != nil {
		return nil, errors.Wrap(err, "Select []HuaweiStorage")
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
		}

		return nil
	}

	return errors.Wrap(err, "Delete Storage By ID:"+id)
}
