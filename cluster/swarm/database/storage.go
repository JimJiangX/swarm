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

func ListLUNByUnitID(id string) ([]LUN, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	list := make([]LUN, 0, 4)
	query := "SELECT * FROM tb_lun WHERE id=?"

	err = db.Select(&list, query, id)
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

func DelLUN(id string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_lun WHERE id=?", id)

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

func DeleteLocalVoume(IDOrName string) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_volumes WHERE id=? OR name=?", IDOrName, IDOrName)

	return err
}

func GetLocalVoume(IDOrName string) (LocalVolume, error) {
	lv := LocalVolume{}

	db, err := GetDB(true)
	if err != nil {
		return lv, err
	}

	query := "SELECT * FROM tb_volumes WHERE id=? OR name=?"

	err = db.Get(&lv, query, IDOrName, IDOrName)
	if err != nil {
		return lv, err
	}

	return lv, nil
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