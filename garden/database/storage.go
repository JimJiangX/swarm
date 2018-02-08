package database

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type StorageIface interface {
	InsertLunSetVolume(lun LUN, lv Volume) error
	InsertLunVolume(lun LUN, lv Volume) error

	LunMapping(lun, host, vg string, hlun int) error
	DelLunMapping(lun string) error

	GetLUN(nameOrID string) (LUN, error)
	GetLunByLunID(systemID string, id int) (LUN, error)

	ListLunByName(name string) ([]LUN, error)

	CountLunByRaidGroupID(rg string) (int, error)

	DelLUN(id string) error
	DelLunVolume(lunID, volume string) error

	ListHostLunIDByMapping(host string) ([]int, error)
	ListLunIDBySystemID(id string) ([]int, error)

	GetRaidGroup(id, rg string) (RaidGroup, error)
	ListRGByStorageID(id string) ([]RaidGroup, error)

	InsertRaidGroup(rg RaidGroup) error

	SetRaidGroupStatus(ssid, rgid string, state bool) error
	SetRGStatusByID(id string, state bool) error

	DelRGCondition(storageID string) error
	DelRaidGroup(id, rg string) error

	InsertSANStorage(hs SANStorage) error

	GetStorageByID(id string) (SANStorage, error)
	ListStorageID() ([]string, error)

	DelStorageByID(id string) error
}

type StorageOrmer interface {
	VolumeOrmer

	StorageIface
}

// LUN is table structure,correspod with SAN storage LUN.
type LUN struct {
	ID              string    `db:"id"`
	Name            string    `db:"name"`
	VG              string    `db:"vg_name"`
	RaidGroupID     string    `db:"raid_group_id"`
	StorageSystemID string    `db:"san_id"`
	MappingTo       string    `db:"mapping_hostname"`
	SizeByte        int       `db:"size"`
	HostLunID       int       `db:"host_lun_id"`
	StorageLunID    int       `db:"san_lun_id"`
	CreatedAt       time.Time `db:"created_at"`
}

func (db dbBase) lunTable() string {
	return db.prefix + "_san_raid_group_lun"
}

func (db dbBase) txInsertLun(tx *sqlx.Tx, lun LUN) error {
	query := "INSERT INTO " + db.lunTable() + " (id,name,vg_name,raid_group_id,san_id,mapping_hostname,size,host_lun_id,san_lun_id,created_at) VALUES (:id,:name,:vg_name,:raid_group_id,:san_id,:mapping_hostname,:size,:host_lun_id,:san_lun_id,:created_at)"
	_, err := tx.NamedExec(query, lun)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "Tx insert LUN")
}

func (db dbBase) InsertLunSetVolume(lun LUN, lv Volume) error {
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

// InsertLunVolume insert LUN and Volume in a Tx,
// the LUN is to creating a Volume
func (db dbBase) InsertLunVolume(lun LUN, lv Volume) error {
	do := func(tx *sqlx.Tx) error {

		err := db.txInsertVolume(tx, lv)
		if err != nil {
			return err
		}

		return db.txInsertLun(tx, lun)
	}

	return db.txFrame(do)
}

func (db dbBase) DelLunVolume(lunID, volume string) error {
	do := func(tx *sqlx.Tx) error {

		query := "DELETE FROM " + db.lunTable() + " WHERE id=?"
		_, err := tx.Exec(query, lunID)
		if err != nil {
			return errors.Wrap(err, "delete LUN by ID")
		}

		return db.txDelVolume(tx, volume)
	}

	return db.txFrame(do)
}

// DelLunMapping delete a mapping record,set LUN VG、MappingTo and HostLunID to be null
func (db dbBase) DelLunMapping(lun string) error {

	query := "UPDATE " + db.lunTable() + " SET vg_name=?,mapping_hostname=? WHERE id=?"

	_, err := db.Exec(query, "", "", lun)
	if err == nil {
		return nil
	}

	return errors.WithStack(err)
}

// LunMapping sets LUN VG、MappingTo、HostLunID value
func (db dbBase) LunMapping(lun, host, vg string, hlun int) error {

	query := "UPDATE " + db.lunTable() + " SET vg_name=?,mapping_hostname=?,host_lun_id=? WHERE id=?"

	_, err := db.Exec(query, vg, host, hlun, lun)
	if err == nil {
		return nil
	}

	return errors.WithStack(err)
}

// GetLUN returns LUN,select by ID
func (db dbBase) GetLUN(nameOrID string) (LUN, error) {
	lun := LUN{}
	query := "SELECT id,name,vg_name,raid_group_id,san_id,mapping_hostname,size,host_lun_id,san_lun_id,created_at FROM " + db.lunTable() + " WHERE id=? OR name=?"

	err := db.Get(&lun, query, nameOrID, nameOrID)
	if err == nil {
		return lun, nil
	}

	return lun, errors.WithStack(err)
}

// ListLunByNameOrVG returns []LUN select by Name or VG
func (db dbBase) ListLunByName(name string) ([]LUN, error) {
	var (
		list  []LUN
		query = "SELECT id,name,vg_name,raid_group_id,san_id,mapping_hostname,size,host_lun_id,san_lun_id,created_at FROM " + db.lunTable() + " WHERE name=?"
	)

	err := db.Select(&list, query, name)
	if err == nil {
		return list, nil
	}

	return nil, errors.Wrap(err, "list []LUN by Name or VG")
}

// GetLunByLunID returns a LUN select by StorageLunID and StorageSystemID
func (db dbBase) GetLunByLunID(systemID string, id int) (LUN, error) {
	lun := LUN{}
	query := "SELECT id,name,vg_name,raid_group_id,san_id,mapping_hostname,size,host_lun_id,san_lun_id,created_at FROM " + db.lunTable() + " WHERE storage_system_id=? AND san_lun_id=?"

	err := db.Get(&lun, query, systemID, id)
	if err == nil {
		return lun, nil
	}

	return lun, errors.Wrap(err, "get LUN by StorageSystemID and StorageLunID")
}

// CountLunByRaidGroupID returns number of result select _lun by RaidGroup
func (db dbBase) CountLunByRaidGroupID(rg string) (int, error) {
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
		query = "SELECT san_lun_id FROM " + db.lunTable() + " WHERE san_id=?"
	)

	err := db.Select(&out, query, id)
	if err == nil {
		return out, nil
	}

	return nil, errors.Wrap(err, "list LUN StorageLunID by StorageSystemID")
}

// RaidGroup is table _raid_group structure,correspod with SNA RaidGroup,
// RG is short of RaidGroup
type RaidGroup struct {
	ID          string `db:"id"`
	StorageID   string `db:"storage_system_id"`
	StorageRGID string `db:"storage_rg_id"`
	Enabled     bool   `db:"enabled"`
}

func (db dbBase) raidGroupTable() string {
	return db.prefix + "_san_raid_group"
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
	query := "SELECT COUNT(id) from " + db.nodeTable() + " WHERE storage=?"

	err := db.Get(&count, query, storageID)
	if err != nil {
		return errors.Wrap(err, "count Node by storage")
	}

	if count > 0 {
		return errors.Errorf("storage is using by %d nodes", count)
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

// SANStorage is table _san structure,
// correspod with HITACHI storage
type SANStorage struct {
	ID        string `db:"id"`
	Vendor    string `db:"vendor"`
	Version   string `db:"version"`
	AdminUnit string `db:"admin_unit"`
	LunStart  int    `db:"lun_start"`
	LunEnd    int    `db:"lun_end"`
	HluStart  int    `db:"hlu_start"`
	HluEnd    int    `db:"hlu_end"`
}

func (db dbBase) sanTable() string {
	return db.prefix + "_san"
}

// InsertSANStorage inserts a new SAN Storage
func (db dbBase) InsertSANStorage(hs SANStorage) error {

	query := "INSERT INTO " + db.sanTable() + " (id,vendor,version,admin_unit,lun_start,lun_end,hlu_start,hlu_end) VALUES (:id,:vendor,:version,:admin_unit,:lun_start,:lun_end,:hlu_start,:hlu_end)"

	_, err := db.NamedExec(query, hs)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "insert HITACHI Storage")
}

// HuaweiStorage is table _storage_HUAWEI structure,
// correspod with HUAWEI storage
//type HuaweiStorage struct {
//	ID       string `db:"id"`
//	Vendor   string `db:"vendor"`
//	Version  string `db:"version"`
//	IPAddr   string `db:"ip_addr"`
//	Username string `db:"username"`
//	Password string `db:"password"`
//	HluStart int    `db:"hlu_start"`
//	HluEnd   int    `db:"hlu_end"`
//}

//func (db dbBase) huaweiTable() string {
//	return db.prefix + "_storage_HUAWEI"
//}

// Insert inserts a new HuaweiStorage
//func (db dbBase) InsertHuaweiStorage(hs HuaweiStorage) error {

//	query := "INSERT INTO " + db.huaweiTable() + " (id,vendor,version,ip_addr,username,password,hlu_start,hlu_end) VALUES (:id,:vendor,:version,:ip_addr,:username,:password,:hlu_start,:hlu_end)"

//	_, err := db.NamedExec(query, hs)
//	if err == nil {
//		return nil
//	}

//	return errors.Wrap(err, "insert HUAWEI Storage")
//}

// GetStorageByID returns *HitachiStorage or *HuaweiStorage,select by ID
func (db dbBase) GetStorageByID(id string) (SANStorage, error) {
	san := SANStorage{}
	query := "SELECT id,vendor,version,admin_unit,lun_start,lun_end,hlu_start,hlu_end FROM " + db.sanTable() + " WHERE id=?"
	err := db.Get(&san, query, id)
	if err == nil {
		return san, nil
	}

	return san, errors.Wrap(err, "not found Storage by ID")
}

// ListStorageID returns all StorageSystemID
func (db dbBase) ListStorageID() ([]string, error) {
	var out []string
	err := db.Select(&out, "SELECT id FROM "+db.sanTable())
	if err != nil {
		return nil, errors.Wrap(err, "select []SANStorage")
	}

	return out, nil
}

// DelStorageByID delete storage system by ID
func (db dbBase) DelStorageByID(id string) error {
	query := "DELETE FROM " + db.sanTable() + " WHERE id=?"

	_, err := db.Exec(query, id)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "delete Storage by ID")
}
