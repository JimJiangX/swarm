package database

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const insertLUNQuery = "INSERT INTO tbl_dbaas_lun (id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at) VALUES (:id,:name,:vg_name,:raid_group_id,:storage_system_id,:mapping_hostname,:size,:host_lun_id,:storage_lun_id,:created_at)"

// LUN is table tbl_dbaas_lun structure,correspod with SAN storage LUN.
type LUN struct {
	ID              string    `db:"id"`
	Name            string    `db:"name"`
	VGName          string    `db:"vg_name"`
	RaidGroupID     string    `db:"raid_group_id"`
	StorageSystemID string    `db:"storage_system_id"`
	MappingTo       string    `db:"mapping_hostname"`
	SizeByte        int       `db:"size"`
	HostLunID       int       `db:"host_lun_id"`
	StorageLunID    int       `db:"storage_lun_id"`
	CreatedAt       time.Time `db:"created_at"`
}

func (l LUN) tableName() string {
	return "tbl_dbaas_lun"
}

func txInsertLun(tx *sqlx.Tx, lun LUN) error {
	_, err := tx.NamedExec(insertLUNQuery, &lun)

	return errors.Wrap(err, "Tx insert LUN")
}

func TxInsertLunUpdateVolume(lun LUN, lv LocalVolume) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txInsertLun(tx, lun)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_volumes SET size=? WHERE id=?"

	_, err = tx.Exec(query, lv.Size, lv.ID)
	if err != nil {
		return err
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx insert LUN and update Volume")
}

// TxInsertLUNAndVolume insert LUN and LocalVolume in a Tx,
// the LUN is to creating a Volume
func TxInsertLUNAndVolume(lun LUN, lv LocalVolume) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = txInsertLun(tx, lun)
	if err != nil {
		return err
	}

	_, err = tx.NamedExec(insertLocalVolumeQuery, &lv)
	if err != nil {
		return errors.Wrap(err, "Tx insert LocalVolume")
	}

	err = tx.Commit()

	return errors.Wrap(err, "Tx insert LUN and Volume")
}

// DelLunMapping delete a mapping record,set LUN VGName、MappingTo and HostLunID to be null
var DelLunMapping = LunMapping

// LunMapping sets LUN VGName、MappingTo、HostLunID value
func LunMapping(lun, host, vgName string, hlun int) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_lun SET vg_name=?,mapping_hostname=?,host_lun_id=? WHERE id=?"

	_, err = db.Exec(query, vgName, host, hlun, lun)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, vgName, host, hlun, lun)

	return errors.Wrap(err, "update LUN Mapping")
}

// GetLUNByID returns LUN,select tbl_dbaas_lun by ID
func GetLUNByID(id string) (LUN, error) {
	db, err := getDB(false)
	if err != nil {
		return LUN{}, err
	}

	lun := LUN{}
	const query = "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM tbl_dbaas_lun WHERE id=? LIMIT 1"

	err = db.Get(&lun, query, id)
	if err == nil {
		return lun, nil
	}

	db, err = getDB(true)
	if err != nil {
		return lun, err
	}

	err = db.Get(&lun, query, id)

	return lun, errors.Wrap(err, "get LUN ny ID")
}

// ListLUNByName returns []LUN select by Name
func ListLUNByName(name string) ([]LUN, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var list []LUN
	const query = "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM tbl_dbaas_lun WHERE name=?"

	err = db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, name)

	return list, errors.Wrap(err, "list []LUN by Name")
}

// ListLUNByVgName returns []LUN select by VGName
func ListLUNByVgName(name string) ([]LUN, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var list []LUN
	const query = "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM tbl_dbaas_lun WHERE vg_name=?"

	err = db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&list, query, name)

	return list, errors.Wrap(err, "list []LUN by VGName")
}

// GetLUNByLunID returns a LUN select by StorageLunID and StorageSystemID
func GetLUNByLunID(systemID string, id int) (LUN, error) {
	db, err := getDB(false)
	if err != nil {
		return LUN{}, err
	}

	lun := LUN{}
	const query = "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM tbl_dbaas_lun WHERE storage_system_id=? AND storage_lun_id=?"

	err = db.Get(&lun, query, systemID, id)
	if err == nil {
		return lun, nil
	}

	db, err = getDB(true)
	if err != nil {
		return lun, err
	}

	err = db.Get(&lun, query, systemID, id)

	return lun, errors.Wrap(err, "get LUN by StorageSystemID and StorageLunID")
}

// CountLUNByRaidGroupID returns number of result select tbl_dbaas_lun by RaidGroup
func CountLUNByRaidGroupID(rg string) (int, error) {
	db, err := getDB(false)
	if err != nil {
		return 0, err
	}

	count := 0
	const query = "SELECT COUNT(id) from tbl_dbaas_lun WHERE raid_group_id=?"

	err = db.Get(&count, query, rg)
	if err == nil {
		return count, nil
	}

	db, err = getDB(true)
	if err != nil {
		return 0, err
	}

	err = db.Get(&count, query, rg)

	return count, errors.Wrap(err, "count LUN by RaidGroupID")
}

// DelLUN delete LUN by ID
func DelLUN(id string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tbl_dbaas_lun WHERE id=?"

	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, id)

	return errors.Wrap(err, "delete LUN by ID")
}

// ListHostLunIDByMapping returns []int select HostLunID by MappingTo
func ListHostLunIDByMapping(host string) ([]int, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var out []int
	const query = "SELECT host_lun_id FROM tbl_dbaas_lun WHERE mapping_hostname=?"

	err = db.Select(&out, query, host)
	if err == nil {
		return out, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, host)

	return out, errors.Wrap(err, "list []LUN HostLunID by MappingTo")
}

// ListLunIDBySystemID returns []int select StorageLunID by StorageSystemID
func ListLunIDBySystemID(id string) ([]int, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var out []int
	const query = "SELECT storage_lun_id FROM tbl_dbaas_lun WHERE storage_system_id=?"

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, id)

	return out, errors.Wrap(err, "list LUN StorageLunID by StorageSystemID")
}

const insertRaidGroupQuery = "INSERT INTO tbl_dbaas_raid_group (id,storage_system_id,storage_rg_id,enabled) VALUES (:id,:storage_system_id,:storage_rg_id,:enabled)"

// RaidGroup is table tbl_dbaas_raid_group structure,correspod with SNA RaidGroup,
// RG is short of RaidGroup
type RaidGroup struct {
	ID          string `db:"id"`
	StorageID   string `db:"storage_system_id"`
	StorageRGID int    `db:"storage_rg_id"`
	Enabled     bool   `db:"enabled"`
}

func (r RaidGroup) tableName() string {
	return "tbl_dbaas_raid_group"
}

// Insert insert a new RaidGroup
func (r RaidGroup) Insert() error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertRaidGroupQuery, &r)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertRaidGroupQuery, &r)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Insert RaidGroup")
}

// UpdateRaidGroupStatus update Enabled select by StorageSystemID and StorageRGID
func UpdateRaidGroupStatus(ssid string, rgid int, state bool) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_raid_group SET enabled=? WHERE storage_system_id=? AND storage_rg_id=?"

	_, err = db.Exec(query, state, ssid, rgid)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, state, ssid, rgid)

	return errors.Wrap(err, "update RaidGroup.Enabled")
}

// UpdateRGStatusByID update Enabled select by ID
func UpdateRGStatusByID(id string, state bool) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_raid_group SET enabled=? WHERE id=?"

	_, err = db.Exec(query, state, id)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, state, id)

	return errors.Wrap(err, "update RaidGroup.Enabled")
}

// ListRGByStorageID returns []RaidGroup select by StorageSystemID
func ListRGByStorageID(id string) ([]RaidGroup, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var out []RaidGroup
	const query = "SELECT id,storage_system_id,storage_rg_id,enabled FROM tbl_dbaas_raid_group WHERE storage_system_id=?"

	err = db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&out, query, id)

	return out, errors.Wrap(err, "list []RaidGroup by StorageSystemID")
}

// GetRaidGroup returns RaidGroup select by StorageSystemID and StorageRGID.
func GetRaidGroup(id string, rg int) (RaidGroup, error) {
	db, err := getDB(false)
	if err != nil {
		return RaidGroup{}, err
	}

	out := RaidGroup{}
	const query = "SELECT id,storage_system_id,storage_rg_id,enabled FROM tbl_dbaas_raid_group WHERE storage_system_id=? AND storage_rg_id=? LIMIT 1"

	err = db.Get(&out, query, id, rg)
	if err == nil {
		return out, nil
	}

	db, err = getDB(true)
	if err != nil {
		return RaidGroup{}, err
	}

	err = db.Get(&out, query, id, rg)

	return out, errors.Wrap(err, "get RaidGroup")
}

// DeleteRaidGroup delete RaidGroup by StorageSystemID and StorageRGID
func DeleteRaidGroup(id string, rg int) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tbl_dbaas_raid_group WHERE storage_system_id=? AND storage_rg_id=?"

	_, err = db.Exec(query, id, rg)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, id, rg)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete RaidGroup")
}

const insertHitachiStorageQuery = "INSERT INTO tbl_dbaas_storage_HITACHI (id,vendor,admin_unit,lun_start,lun_end,hlu_start,hlu_end) VALUES (:id,:vendor,:admin_unit,:lun_start,:lun_end,:hlu_start,:hlu_end)"

// HitachiStorage is table tbl_dbaas_storage_HITACHI structure,
// correspod with HITACHI storage
type HitachiStorage struct {
	ID        string `db:"id"`
	Vendor    string `db:"vendor"`
	AdminUnit string `db:"admin_unit"`
	LunStart  int    `db:"lun_start"`
	LunEnd    int    `db:"lun_end"`
	HluStart  int    `db:"hlu_start"`
	HluEnd    int    `db:"hlu_end"`
}

func (HitachiStorage) tableName() string {
	return "tbl_dbaas_storage_HITACHI"
}

// Insert inserts a new HitachiStorage
func (hs HitachiStorage) Insert() error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHitachiStorageQuery, &hs)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHitachiStorageQuery, &hs)

	return errors.Wrap(err, "insert HITACHI Storage")
}

const insertHuaweiStorageQuery = "INSERT INTO tbl_dbaas_storage_HUAWEI (id,vendor,ip_addr,username,password,hlu_start,hlu_end) VALUES (:id,:vendor,:ip_addr,:username,:password,:hlu_start,:hlu_end)"

// HuaweiStorage is table tbl_dbaas_storage_HUAWEI structure,
// correspod with HUAWEI storage
type HuaweiStorage struct {
	ID       string `db:"id"`
	Vendor   string `db:"vendor"`
	IPAddr   string `db:"ip_addr"`
	Username string `db:"username"`
	Password string `db:"password"`
	HluStart int    `db:"hlu_start"`
	HluEnd   int    `db:"hlu_end"`
}

func (HuaweiStorage) tableName() string {
	return "tbl_dbaas_storage_HUAWEI"
}

// Insert inserts a new HuaweiStorage
func (hs HuaweiStorage) Insert() error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHuaweiStorageQuery, &hs)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertHuaweiStorageQuery, &hs)

	return errors.Wrap(err, "insert HUAWEI Storage")
}

const insertLocalVolumeQuery = "INSERT INTO tbl_dbaas_volumes (id,name,unit_id,size,VGname,driver,fstype) VALUES (:id,:name,:unit_id,:size,:VGname,:driver,:fstype)"

// LocalVolume is table tbl_dbaas_volumes structure,
// correspod with host LV
type LocalVolume struct {
	Size       int    `db:"size"`
	ID         string `db:"id"`
	Name       string `db:"name"`
	UnitID     string `db:"unit_id"`
	VGName     string `db:"VGname"`
	Driver     string `db:"driver"`
	Filesystem string `db:"fstype"`
}

func (LocalVolume) tableName() string {
	return "tbl_dbaas_volumes"
}

// InsertLocalVolume insert a new LocalVolume
func InsertLocalVolume(lv LocalVolume) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertLocalVolumeQuery, &lv)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.NamedExec(insertLocalVolumeQuery, &lv)

	return errors.Wrap(err, "insert LocalVolume")
}

// UpdateLocalVolume update size of LocalVolume by name or ID
func UpdateLocalVolume(nameOrID string, size int) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "UPDATE tbl_dbaas_volumes SET size=? WHERE id=? OR name=?"

	_, err = db.Exec(query, size, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, size, nameOrID, nameOrID)

	return errors.Wrap(err, "update LocalVolume size")
}

// TxUpdateMultiLocalVolume update Size of LocalVolume by name or ID in a Tx
func TxUpdateMultiLocalVolume(lvs []LocalVolume) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Preparex("UPDATE tbl_dbaas_volumes SET size=? WHERE id=?")
	if err != nil {
		return errors.Wrap(err, "Tx prepare update local Volume")
	}

	for _, lv := range lvs {
		_, err := stmt.Exec(lv.Size, lv.ID)
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx update LocalVolume size")
		}
	}

	stmt.Close()

	err = tx.Commit()

	return errors.Wrap(err, "Tx update LocalVolume size")
}

// DeleteLocalVoume delete LocalVolume by name or ID
func DeleteLocalVoume(nameOrID string) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	const query = "DELETE FROM tbl_dbaas_volumes WHERE id=? OR name=?"

	_, err = db.Exec(query, nameOrID, nameOrID)
	if err == nil {
		return nil
	}

	db, err = getDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec(query, nameOrID, nameOrID)

	return errors.Wrap(err, "delete LocalVolume by nameOrID")
}

// TxDeleteVolume delete LocalVolume by name or ID or UnitID
func TxDeleteVolume(tx *sqlx.Tx, nameOrID string) error {
	_, err := tx.Exec("DELETE FROM tbl_dbaas_volumes WHERE id=? OR name=? OR unit_id=?", nameOrID, nameOrID, nameOrID)

	return errors.Wrap(err, "Tx delete LocalVolume")
}

// TxDeleteVolumes delete []LocalVoume in a Tx.
func TxDeleteVolumes(volumes []LocalVolume) error {
	tx, err := GetTX()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	stmt, err := tx.Preparex("DELETE FROM tbl_dbaas_volumes WHERE id=?")
	if err != nil {
		return errors.Wrap(err, "Tx prepare delete []LocalVolume")
	}

	for i := range volumes {
		_, err = stmt.Exec(volumes[i].ID)
		if err != nil {
			stmt.Close()

			return errors.Wrap(err, "Tx delete LocalVolume:"+volumes[i].ID)
		}
	}

	stmt.Close()

	err = tx.Commit()

	return errors.Wrap(err, "Tx delete []LocalVolume")
}

// GetLocalVolume returns LocalVolume select by name or ID
func GetLocalVolume(nameOrID string) (LocalVolume, error) {
	lv := LocalVolume{}

	db, err := getDB(false)
	if err != nil {
		return lv, err
	}

	const query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM tbl_dbaas_volumes WHERE id=? OR name=?"

	err = db.Get(&lv, query, nameOrID, nameOrID)
	if err == nil {
		return lv, nil
	}

	db, err = getDB(true)
	if err != nil {
		return lv, err
	}

	err = db.Get(&lv, query, nameOrID, nameOrID)

	return lv, errors.Wrap(err, "get LocalVolume by nameOrID")
}

// ListVolumeByVG returns []LocalVolume select by VGName
func ListVolumeByVG(name string) ([]LocalVolume, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var lvs []LocalVolume
	const query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM tbl_dbaas_volumes WHERE VGname=?"

	err = db.Select(&lvs, query, name)
	if err == nil {
		return lvs, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&lvs, query, name)

	return lvs, errors.Wrap(err, "list []LocalVolume by VGName")
}

// ListVolumesByUnitID returns []LocalVolume select by UnitID
func ListVolumesByUnitID(id string) ([]LocalVolume, error) {
	db, err := getDB(false)
	if err != nil {
		return nil, err
	}

	var lvs []LocalVolume
	const query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM tbl_dbaas_volumes WHERE unit_id=?"

	err = db.Select(&lvs, query, id)
	if err == nil {
		return lvs, nil
	}

	db, err = getDB(true)
	if err != nil {
		return nil, err
	}

	err = db.Select(&lvs, query, id)

	return lvs, errors.Wrap(err, "list []LocalVolume by UnitID")
}

// GetStorageByID returns *HitachiStorage or *HuaweiStorage,select by ID
func GetStorageByID(id string) (*HitachiStorage, *HuaweiStorage, error) {
	db, err := getDB(true)
	if err != nil {
		return nil, nil, err
	}

	hitachi, huawei := &HitachiStorage{}, &HuaweiStorage{}

	err = db.Get(hitachi, "SELECT id,vendor,admin_unit,lun_start,lun_end,hlu_start,hlu_end FROM tbl_dbaas_storage_HITACHI WHERE id=?", id)
	if err == nil {
		return hitachi, nil, nil
	}

	err = db.Get(huawei, "SELECT id,vendor,ip_addr,username,password,hlu_start,hlu_end FROM tbl_dbaas_storage_HUAWEI WHERE id=?", id)
	if err == nil {
		return nil, huawei, nil
	}

	return nil, nil, errors.Wrap(err, "not Ffound Storage by ID")
}

// ListStorageID returns all StorageSystemID
func ListStorageID() ([]string, error) {
	db, err := getDB(true)
	if err != nil {
		return nil, err
	}

	var hitachi []string
	err = db.Select(&hitachi, "SELECT id FROM tbl_dbaas_storage_HITACHI")
	if err != nil {
		return nil, errors.Wrap(err, "select []HitachiStorage")
	}

	var huawei []string
	err = db.Select(&huawei, "SELECT id FROM tbl_dbaas_storage_HUAWEI")
	if err != nil {
		return nil, errors.Wrap(err, "select []HuaweiStorage")
	}

	out := make([]string, len(hitachi)+len(huawei))

	length := copy(out, hitachi)
	copy(out[length:], huawei)

	return out, nil
}

// DeleteStorageByID delete storage system by ID
func DeleteStorageByID(id string) error {
	db, err := getDB(true)
	if err != nil {
		return err
	}

	r, err := db.Exec("DELETE FROM tbl_dbaas_storage_HITACHI WHERE id=?", id)
	if err == nil {
		num, err := r.RowsAffected()
		if num > 0 && err == nil {
			return nil
		}
	}

	_, err = db.Exec("DELETE FROM tbl_dbaas_storage_HUAWEI WHERE id=?", id)

	return errors.Wrap(err, "delete Storage by ID")
}
