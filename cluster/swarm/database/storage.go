package database

import "time"

type LUN struct {
	ID              string    `db:"id"`
	Name            string    `db:"name"`
	UnitID          string    `db:"unit_id"`
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

func InsertLUN(lun LUN) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "INSERT INTO tb_lun (id,name,unit_id,raid_group_id,storage_system_id,mapping_hostname,size,host_lun_id,storage_lun_id,created_at) VALUES (:id,:name,:unit_id,:raid_group_id,:storage_system_id,:mapping_hostname,:size,:host_lun_id,:storage_lun_id,:created_at)"

	_, err = db.NamedExec(query, &lun)

	return err
}

var DelLunMapping = LunMapping

func LunMapping(lun, host, unit string, hlun int) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "UPDATE tb_lun SET unit_id=?,mapping_hostname=?,host_lun_id=? WHERE id=?"

	_, err = db.Exec(query, unit, host, hlun, lun)

	return err
}

func GetLUNByID(id string) (LUN, error) {
	db, err := GetDB(true)
	if err != nil {
		return LUN{}, err
	}

	var lun LUN
	query := "SELECT * FROM tb_lun WHRER id=? LIMIT 1"

	err = db.QueryRowx(query, id).StructScan(&lun)

	return lun, err
}

func GetLUNByLunID(systemID string, id int) (LUN, error) {
	db, err := GetDB(true)
	if err != nil {
		return LUN{}, err
	}

	var lun LUN
	query := "SELECT * FROM tb_lun WHRER storage_system_id=? AND storage_lun_id=? LIMIT 1"

	err = db.QueryRowx(query, systemID, id).StructScan(&lun)

	return lun, err
}

func DelLUN(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE tb_lun WHERE id=?", id)

	return err
}

func SelectHostLunIDByMapping(host string) ([]int, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var out []int
	query := "SELECT host_lun_id FROM tb_lun WHERE mapping_hostname=?"

	err = db.Select(&out, query, host)

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

	return out, err
}

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

	query := "INSERT INTO tb_raid_group (id,storage_system_id,storage_rg_id,enabled) VALUES (:id,:storage_system_id,:storage_rg_id,:enabled)"

	_, err = db.NamedExec(query, &rg)

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

func SelectRaidGroupByStorageID(id string, state bool) ([]*RaidGroup, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	var out []*RaidGroup
	query := "SELECT * FROM tb_raid_group WHERE storage_system_id=? AND enabled=?"

	err = db.Select(&out, query, id, state)

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

func (hs *HitachiStorage) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "INSERT INTO tb_storage_HITACHI (id,vendor,admin_unit,lun_start,lun_end,hlu_start,hlu_end) VALUES (:id,:vendor,:admin_unit,:lun_start,:lun_end,:hlu_start,:hlu_end)"
	_, err = db.NamedExec(query, hs)

	return err
}

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

func (hs *HuaweiStorage) Insert() error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	query := "INSERT INTO tb_storage_HUAWEI (id,vendor,ip_addr,username,password,hlu_start,hlu_end) VALUES (:id,:vendor,:ip_addr,:username,:password,:hlu_start,:hlu_end)"
	_, err = db.NamedExec(query, hs)

	return err
}

type LocalVolume struct {
	ID         string `db:"id"`
	Name       string `db:"name"`
	Size       int    `db:"size"`
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

	query := "INSERT INTO tb_volumes (id,name,size,VGname,driver,fstype) VALUES (:id,:name,:size,:VGname,:driver,:fstype)"
	_, err = db.NamedExec(query, &lv)

	return err
}

func SelectVolumeByVG(name string) ([]LocalVolume, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	lvs := []LocalVolume{}
	query := "SELECT * FROM tb_volumes WHERE VGname=?"

	err = db.Select(&lvs, query, name)
	if err != nil {
		return nil, err
	}

	return lvs, nil
}

/*
type Volume struct {
	Name     string `db:"id"`
	Type     string `db:"type"`
	SizeByte int64  `db:"size"`
	NodeID   string `db:"node_id"`
	LunID    string `db:"lun_id"`
	VGName   string `db:"vg_name"`
}

func (v Volume) TableName() string {
	return "tb_volume"
}

func TxInsertMultiVolume(tx *sqlx.Tx, volumes []*Volume) error {
	query := "INSERT INTO tb_volume (id,type,size,node_id,lun_id,vg_name) VALUES (:id,:type,:size,:node_id,:lun_id,:vg_name)"

	stmt, err := tx.PrepareNamed(query)
	if err != nil {
		return err
	}

	for i := range volumes {
		if volumes[i] == nil {
			continue
		}

		_, err = stmt.Exec(volumes[i])
		if err != nil {
			return err
		}
	}

	return nil
}
*/
