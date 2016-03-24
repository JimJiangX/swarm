package database

import "time"

type LUN struct {
	ID           string    `db:"id"`
	Name         int       `db:"name"`
	UnitID       string    `db:"unit_id"`
	StorageLunID int       `db:"storage_lun_id"`
	RaidGroupID  int       `db:"raid_groups_id"`
	Mappingto    string    `db:"mapping_to"`
	SizeByte     int64     `db:"size"`
	HostLunID    int64     `db:"host_lun_id"`
	CreatedAt    time.Time `db:"created_at"`
}

func (l LUN) TableName() string {
	return "tb_lun"
}

type RaidGroup struct {
	ID          string `db:"id"`
	StorageID   string `db:"storage_system_id"`
	StorageRGID string `db:"storage_rg_id"`
	Enabled     bool   `db:"enabled"`
}

func (r RaidGroup) TableName() string {
	return "tb_raid_group"
}

type HDSStorage struct {
	ID        string `db:"id"`
	Vendor    string `db:"vendor"`
	AdminUnit string `db:"admin_unit"`
	LunStart  int    `db:"lun_start"`
	LunEnd    int    `db:"lun_end"`
	HluStart  int    `db:"hlu_start"`
	HluEnd    int    `db:"hlu_end"`
}

func (hds HDSStorage) TableName() string {
	return "tb_storage_HITACHI"
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
