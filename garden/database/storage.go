package database

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// const insertLUNQuery = "INSERT INTO tbl_dbaas_lun (id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at) VALUES (:id,:name,:vg_name,:raid_group_id,:storage_system_id,:mapping_hostname,:size,:host_lun_id,:storage_lun_id,:created_at)"

// LUN is table structure,correspod with SAN storage LUN.
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

func (db dbBase) lunTable() string {
	return db.prefix + "_lun"
}

func (db dbBase) txInsertLun(tx *sqlx.Tx, lun LUN) error {
	query := "INSERT INTO " + db.lunTable() + " (id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at) VALUES (:id,:name,:vg_name,:raid_group_id,:storage_system_id,:mapping_hostname,:size,:host_lun_id,:storage_lun_id,:created_at)"
	_, err := tx.NamedExec(query, lun)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx insert LUN")
}

func (db dbBase) InsertLunUpdateVolume(lun LUN, lv Volume) error {
	do := func(tx *sqlx.Tx) error {

		err := db.txInsertLun(tx, lun)
		if err != nil {
			return err
		}

		query := "UPDATE " + db.volumeTable() + " SET size=? WHERE id=?"

		_, err = tx.Exec(query, lv.Size, lv.ID)
		if err == nil {
			return nil
		}

		return errors.WithStack(err)
	}

	return db.txFrame(do)
}

// InsertLUNAndVolume insert LUN and Volume in a Tx,
// the LUN is to creating a Volume
func (db dbBase) InsertLUNVolume(lun LUN, lv Volume) error {
	do := func(tx *sqlx.Tx) error {

		err := db.txInsertLun(tx, lun)
		if err != nil {
			return err
		}

		return db.txInsertVolume(tx, lv)
	}

	return db.txFrame(do)
}

// DelLunMapping delete a mapping record,set LUN VGName、MappingTo and HostLunID to be null
func (db dbBase) DelLunMapping(lun, host, vgName string, hlun int) error {

	return db.LunMapping(lun, host, vgName, hlun)
}

// LunMapping sets LUN VGName、MappingTo、HostLunID value
func (db dbBase) LunMapping(lun, host, vgName string, hlun int) error {

	query := "UPDATE " + db.lunTable() + " SET vg_name=?,mapping_hostname=?,host_lun_id=? WHERE id=?"

	_, err := db.Exec(query, vgName, host, hlun, lun)
	if err == nil {
		return nil
	}

	return errors.WithStack(err)
}

// GetLUNByID returns LUN,select by ID
func (db dbBase) GetLUN(ID string) (LUN, error) {
	lun := LUN{}
	query := "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM " + db.lunTable() + " WHERE id=?"

	err := db.Get(&lun, query, ID)
	if err == nil {
		return lun, nil
	}

	return lun, errors.WithStack(err)
}

// ListLUNByName returns []LUN select by Name
func (db dbBase) ListLUNByName(name string) ([]LUN, error) {
	var (
		list  []LUN
		query = "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM " + db.lunTable() + " WHERE name=?"
	)

	err := db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "list []LUN by Name")
}

// ListLUNByVG returns []LUN select by VGName
func (db dbBase) ListLUNByVG(vg string) ([]LUN, error) {
	var (
		list  []LUN
		query = "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM " + db.lunTable() + " WHERE vg_name=?"
	)

	err := db.Select(&list, query, vg)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "list []LUN by VG")
}

// GetLUNByLunID returns a LUN select by StorageLunID and StorageSystemID
func (db dbBase) GetLUNByLunID(systemID string, id int) (LUN, error) {
	lun := LUN{}
	query := "SELECT id,name,vg_name,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at FROM " + db.lunTable() + " WHERE storage_system_id=? AND storage_lun_id=?"

	err := db.Get(&lun, query, systemID, id)
	if err == nil {
		return lun, nil
	}

	return lun, errors.Wrap(err, "get LUN by StorageSystemID and StorageLunID")
}

// CountLUNByRaidGroupID returns number of result select _lun by RaidGroup
func (db dbBase) CountLUNByRaidGroupID(rg string) (int, error) {
	count := 0
	query := "SELECT COUNT(id) FROM " + db.lunTable() + " WHERE raid_group_id=?"

	err := db.Get(&count, query, rg)
	if err == nil {
		return count, nil
	}

	return 0, errors.Wrap(err, "count LUN by RaidGroupID")
}

// DelLUN delete LUN by ID
func (db dbBase) DelLUN(id string) error {

	query := "DELETE FROM " + db.lunTable() + " WHERE id=?"

	_, err := db.Exec(query, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "delete LUN by ID")
}

// ListHostLunIDByMapping returns []int select HostLunID by MappingTo
func (db dbBase) ListHostLunIDByMapping(host string) ([]int, error) {
	var (
		out   []int
		query = "SELECT host_lun_id FROM " + db.lunTable() + " WHERE mapping_hostname=?"
	)

	err := db.Select(&out, query, host)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "list []LUN HostLunID by MappingTo")
}

// ListLunIDBySystemID returns []int select StorageLunID by StorageSystemID
func (db dbBase) ListLunIDBySystemID(id string) ([]int, error) {
	var (
		out   []int
		query = "SELECT storage_lun_id FROM " + db.lunTable() + " WHERE storage_system_id=?"
	)

	err := db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "list LUN StorageLunID by StorageSystemID")
}

// const insertRaidGroupQuery = "INSERT INTO tbl_dbaas_raid_group (id,storage_system_id,storage_rg_id,enabled) VALUES (:id,:storage_system_id,:storage_rg_id,:enabled)"

// RaidGroup is table _raid_group structure,correspod with SNA RaidGroup,
// RG is short of RaidGroup
type RaidGroup struct {
	ID          string `db:"id"`
	StorageID   string `db:"storage_system_id"`
	StorageRGID string `db:"storage_rg_id"`
	Enabled     bool   `db:"enabled"`
}

func (db dbBase) raidGroupTable() string {
	return db.prefix + "_raid_group"
}

// InsertRaidGroup insert a new RaidGroup
func (db dbBase) InsertRaidGroup(rg RaidGroup) error {
	query := "INSERT INTO " + db.raidGroupTable() + " (id,storage_system_id,storage_rg_id,enabled) VALUES (:id,:storage_system_id,:storage_rg_id,:enabled)"

	_, err := db.NamedExec(query, rg)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "insert RaidGroup")
}

// SetRaidGroupStatus update Enabled select by StorageSystemID and StorageRGID
func (db dbBase) SetRaidGroupStatus(ssid, rgid string, state bool) error {

	query := "UPDATE " + db.raidGroupTable() + " SET enabled=? WHERE storage_system_id=? AND storage_rg_id=?"

	_, err := db.Exec(query, state, ssid, rgid)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "update RaidGroup.Enabled")
}

// SetRGStatusByID update Enabled select by ID
func (db dbBase) SetRGStatusByID(id string, state bool) error {

	query := "UPDATE " + db.raidGroupTable() + " SET enabled=? WHERE id=?"

	_, err := db.Exec(query, state, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "update RaidGroup.Enabled")
}

// ListRGByStorageID returns []RaidGroup select by StorageSystemID
func (db dbBase) ListRGByStorageID(id string) ([]RaidGroup, error) {
	var (
		out   []RaidGroup
		query = "SELECT id,storage_system_id,storage_rg_id,enabled FROM " + db.raidGroupTable() + " WHERE storage_system_id=?"
	)

	err := db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "list []RaidGroup by StorageSystemID")
}

// GetRaidGroup returns RaidGroup select by StorageSystemID and StorageRGID.
func (db dbBase) GetRaidGroup(id, rg string) (RaidGroup, error) {
	r := RaidGroup{}
	query := "SELECT id,storage_system_id,storage_rg_id,enabled FROM " + db.raidGroupTable() + " WHERE storage_system_id=? AND storage_rg_id=? LIMIT 1"

	err := db.Get(&r, query, id, rg)
	if err == nil {
		return r, nil
	}

	return r, errors.Wrap(err, "get RaidGroup")
}

// DelRGCondition count Clusters by storageID.
func (db dbBase) DelRGCondition(storageID string) error {
	count := 0
	query := "SELECT COUNT(id) from " + db.raidGroupTable() + " WHERE storage_id=?"

	err := db.Get(&count, query, storageID)
	if err != nil {
		return errors.Wrap(err, "count Cluster by storage_id")
	}

	if count > 0 {
		return errors.Errorf("storage is using by %d clusters", count)
	}

	query = "SELECT COUNT(id) from " + db.raidGroupTable() + " WHERE storage_system_id=?"

	err = db.Get(&count, query, storageID)
	if err != nil {
		return errors.Wrap(err, "count RaidGroup by storage_system_id")
	}

	if count > 0 {
		return errors.Errorf("storage is using by %d RaidGroup", count)
	}

	return nil
}

// DelRaidGroup delete RaidGroup by StorageSystemID and StorageRGID
func (db dbBase) DelRaidGroup(id, rg string) error {

	query := "DELETE FROM " + db.raidGroupTable() + " WHERE storage_system_id=? AND storage_rg_id=?"

	_, err := db.Exec(query, id, rg)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Delete RaidGroup")
}

// const insertHitachiStorageQuery = "INSERT INTO tbl_dbaas_storage_HITACHI (id,vendor,admin_unit,lun_start,lun_end,hlu_start,hlu_end) VALUES (:id,:vendor,:admin_unit,:lun_start,:lun_end,:hlu_start,:hlu_end)"

// HitachiStorage is table _storage_HITACHI structure,
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

func (db dbBase) hitachiTable() string {
	return db.prefix + "_storage_HITACHI"
}

// Insert inserts a new HitachiStorage
func (db dbBase) InsertHitachiStorage(hs HitachiStorage) error {

	query := "INSERT INTO " + db.hitachiTable() + " (id,vendor,admin_unit,lun_start,lun_end,hlu_start,hlu_end) VALUES (:id,:vendor,:admin_unit,:lun_start,:lun_end,:hlu_start,:hlu_end)"

	_, err := db.NamedExec(query, hs)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "insert HITACHI Storage")
}

// const insertHuaweiStorageQuery = "INSERT INTO tbl_dbaas_storage_HUAWEI (id,vendor,ip_addr,username,password,hlu_start,hlu_end) VALUES (:id,:vendor,:ip_addr,:username,:password,:hlu_start,:hlu_end)"

// HuaweiStorage is table _storage_HUAWEI structure,
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

func (db dbBase) huaweiTable() string {
	return db.prefix + "_storage_HUAWEI"
}

// Insert inserts a new HuaweiStorage
func (db dbBase) InsertHuaweiStorage(hs HuaweiStorage) error {

	query := "INSERT INTO " + db.huaweiTable() + " (id,vendor,ip_addr,username,password,hlu_start,hlu_end) VALUES (:id,:vendor,:ip_addr,:username,:password,:hlu_start,:hlu_end)"

	_, err := db.NamedExec(query, hs)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "insert HUAWEI Storage")
}

//const insertLocalVolumeQuery = "INSERT INTO tbl_dbaas_volumes (id,name,unit_id,size,VGname,driver,fstype) VALUES (:id,:name,:unit_id,:size,:VGname,:driver,:fstype)"

//// LocalVolume is table tbl_dbaas_volumes structure,
//// correspod with host LV
//type LocalVolume struct {
//	Size       int    `db:"size"`
//	ID         string `db:"id"`
//	Name       string `db:"name"`
//	UnitID     string `db:"unit_id"`
//	VGName     string `db:"VGname"`
//	Driver     string `db:"driver"`
//	Filesystem string `db:"fstype"`
//}

//func (LocalVolume) tableName() string {
//	return "tbl_dbaas_volumes"
//}

//// InsertLocalVolume insert a new LocalVolume
//func InsertLocalVolume(lv LocalVolume) error {
//	db, err := getDB(false)
//	if err != nil {
//		return err
//	}

//	_, err = db.NamedExec(insertLocalVolumeQuery, &lv)
//	if err == nil {
//		return nil
//	}

//	db, err = getDB(true)
//	if err != nil {
//		return err
//	}

//	_, err = db.NamedExec(insertLocalVolumeQuery, &lv)

//	return errors.Wrap(err, "insert LocalVolume")
//}

//// UpdateLocalVolume update size of LocalVolume by name or ID
//func UpdateLocalVolume(nameOrID string, size int) error {
//	db, err := getDB(false)
//	if err != nil {
//		return err
//	}

//	const query = "UPDATE tbl_dbaas_volumes SET size=? WHERE id=? OR name=?"

//	_, err = db.Exec(query, size, nameOrID, nameOrID)
//	if err == nil {
//		return nil
//	}

//	db, err = getDB(true)
//	if err != nil {
//		return err
//	}

//	_, err = db.Exec(query, size, nameOrID, nameOrID)

//	return errors.Wrap(err, "update LocalVolume size")
//}

//// TxUpdateMultiLocalVolume update Size of LocalVolume by name or ID in a Tx
//func TxUpdateMultiLocalVolume(lvs []LocalVolume) error {
//	tx, err := GetTX()
//	if err != nil {
//		return err
//	}
//	defer tx.Rollback()

//	stmt, err := tx.Preparex("UPDATE tbl_dbaas_volumes SET size=? WHERE id=?")
//	if err != nil {
//		return errors.Wrap(err, "Tx prepare update local Volume")
//	}

//	for _, lv := range lvs {
//		_, err := stmt.Exec(lv.Size, lv.ID)
//		if err != nil {
//			stmt.Close()

//			return errors.Wrap(err, "Tx update LocalVolume size")
//		}
//	}

//	stmt.Close()

//	err = tx.Commit()

//	return errors.Wrap(err, "Tx update LocalVolume size")
//}

//// DeleteLocalVoume delete LocalVolume by name or ID
//func DeleteLocalVoume(nameOrID string) error {
//	db, err := getDB(false)
//	if err != nil {
//		return err
//	}

//	const query = "DELETE FROM tbl_dbaas_volumes WHERE id=? OR name=?"

//	_, err = db.Exec(query, nameOrID, nameOrID)
//	if err == nil {
//		return nil
//	}

//	db, err = getDB(true)
//	if err != nil {
//		return err
//	}

//	_, err = db.Exec(query, nameOrID, nameOrID)

//	return errors.Wrap(err, "delete LocalVolume by nameOrID")
//}

//// TxDeleteVolume delete LocalVolume by name or ID or UnitID
//func TxDeleteVolume(tx *sqlx.Tx, nameOrID string) error {
//	_, err := tx.Exec("DELETE FROM tbl_dbaas_volumes WHERE id=? OR name=? OR unit_id=?", nameOrID, nameOrID, nameOrID)

//	return errors.Wrap(err, "Tx delete LocalVolume")
//}

//// TxDeleteVolumes delete []LocalVoume in a Tx.
//func TxDeleteVolumes(volumes []LocalVolume) error {
//	tx, err := GetTX()
//	if err != nil {
//		return err
//	}

//	defer tx.Rollback()

//	stmt, err := tx.Preparex("DELETE FROM tbl_dbaas_volumes WHERE id=?")
//	if err != nil {
//		return errors.Wrap(err, "Tx prepare delete []LocalVolume")
//	}

//	for i := range volumes {
//		_, err = stmt.Exec(volumes[i].ID)
//		if err != nil {
//			stmt.Close()

//			return errors.Wrap(err, "Tx delete LocalVolume:"+volumes[i].ID)
//		}
//	}

//	stmt.Close()

//	err = tx.Commit()

//	return errors.Wrap(err, "Tx delete []LocalVolume")
//}

//// GetLocalVolume returns LocalVolume select by name or ID
//func GetLocalVolume(nameOrID string) (LocalVolume, error) {
//	lv := LocalVolume{}

//	db, err := getDB(false)
//	if err != nil {
//		return lv, err
//	}

//	const query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM tbl_dbaas_volumes WHERE id=? OR name=?"

//	err = db.Get(&lv, query, nameOrID, nameOrID)
//	if err == nil {
//		return lv, nil
//	}

//	db, err = getDB(true)
//	if err != nil {
//		return lv, err
//	}

//	err = db.Get(&lv, query, nameOrID, nameOrID)

//	return lv, errors.Wrap(err, "get LocalVolume by nameOrID")
//}

//// ListVolumeByVG returns []LocalVolume select by VGName
//func ListVolumeByVG(name string) ([]LocalVolume, error) {
//	db, err := getDB(false)
//	if err != nil {
//		return nil, err
//	}

//	var lvs []LocalVolume
//	const query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM tbl_dbaas_volumes WHERE VGname=?"

//	err = db.Select(&lvs, query, name)
//	if err == nil {
//		return lvs, nil
//	}

//	db, err = getDB(true)
//	if err != nil {
//		return nil, err
//	}

//	err = db.Select(&lvs, query, name)

//	return lvs, errors.Wrap(err, "list []LocalVolume by VGName")
//}

//// ListVolumesByUnitID returns []LocalVolume select by UnitID
//func ListVolumesByUnitID(id string) ([]LocalVolume, error) {
//	db, err := getDB(false)
//	if err != nil {
//		return nil, err
//	}

//	var lvs []LocalVolume
//	const query = "SELECT id,name,unit_id,size,VGname,driver,fstype FROM tbl_dbaas_volumes WHERE unit_id=?"

//	err = db.Select(&lvs, query, id)
//	if err == nil {
//		return lvs, nil
//	}

//	db, err = getDB(true)
//	if err != nil {
//		return nil, err
//	}

//	err = db.Select(&lvs, query, id)

//	return lvs, errors.Wrap(err, "list []LocalVolume by UnitID")
//}

// GetStorageByID returns *HitachiStorage or *HuaweiStorage,select by ID
func (db dbBase) GetStorageByID(id string) (*HitachiStorage, *HuaweiStorage, error) {
	hitachi, huawei := &HitachiStorage{}, &HuaweiStorage{}

	query := "SELECT id,vendor,admin_unit,lun_start,lun_end,hlu_start,hlu_end FROM " + db.hitachiTable() + " WHERE id=?"
	err := db.Get(hitachi, query, id)
	if err == nil {
		return hitachi, nil, nil
	}

	query = "SELECT id,vendor,ip_addr,username,password,hlu_start,hlu_end FROM " + db.huaweiTable() + " WHERE id=?"
	err = db.Get(huawei, query, id)
	if err == nil {
		return nil, huawei, nil
	}

	return nil, nil, errors.Wrap(err, "not found Storage by ID")
}

// ListStorageID returns all StorageSystemID
func (db dbBase) ListStorageID() ([]string, error) {
	var hitachi []string
	err := db.Select(&hitachi, "SELECT id FROM "+db.hitachiTable())
	if err != nil {
		return nil, errors.Wrap(err, "select []HitachiStorage")
	}

	var huawei []string
	err = db.Select(&huawei, "SELECT id FROM "+db.huaweiTable())
	if err != nil {
		return nil, errors.Wrap(err, "select []HuaweiStorage")
	}

	out := make([]string, len(hitachi)+len(huawei))

	length := copy(out, hitachi)
	copy(out[length:], huawei)

	return out, nil
}

// DelStorageByID delete storage system by ID
func (db dbBase) DelStorageByID(id string) error {
	query := "DELETE FROM " + db.hitachiTable() + " WHERE id=?"

	r, err := db.Exec(query, id)
	if err == nil {
		num, err := r.RowsAffected()
		if num > 0 && err == nil {
			return nil
		}
	}

	query = "DELETE FROM " + db.huaweiTable() + " WHERE id=?"
	_, err = db.Exec(query, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "delete Storage by ID")
}
