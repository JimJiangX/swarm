package database

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func insertLUN(t *testing.T, lun LUN) error {
	db, err := GetDB(true)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.NamedExec(insertLUNQuery, &lun)
	if err != nil {
		t.Fatal(err)
	}

	return err
}

func TestLun(t *testing.T) {
	lun := LUN{
		ID:              "lunId001",
		Name:            "lunName001",
		VGName:          "lunName001_VG",
		RaidGroupID:     "lunRaidGroupId001",
		StorageSystemID: "lunStorageSystemId001",
		Mappingto:       "lunMapingto001",
		SizeByte:        1,
		HostLunID:       1,
		StorageLunID:    1,
		CreatedAt:       time.Now(),
	}
	err := insertLUN(t, lun)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := DelLUN(lun.ID)
		if err != nil {
			t.Fatal(err)
		}
	}()

	host := "lunMapingto099"
	hlun := 99
	vgName := ""
	lun.Mappingto = host
	lun.VGName = ""
	lun.HostLunID = hlun
	err = LunMapping(lun.ID, host, vgName, hlun)
	if err != nil {
		t.Fatal(err)
	}

	lun1, err := GetLUNByID(lun.ID)
	b, _ := json.MarshalIndent(&lun, "", "  ")
	b1, _ := json.MarshalIndent(&lun1, "", "  ")
	if lun.CreatedAt.Format("2006-01-02 15:04:05") != lun1.CreatedAt.Format("2006-01-02 15:04:05") ||
		lun.HostLunID != lun1.HostLunID ||
		lun.ID != lun1.ID ||
		lun.Mappingto != lun1.Mappingto ||
		lun.Name != lun1.Name ||
		lun.RaidGroupID != lun1.RaidGroupID ||
		lun.SizeByte != lun1.SizeByte ||
		lun.StorageLunID != lun1.StorageLunID ||
		lun.StorageSystemID != lun1.StorageSystemID ||
		lun.VGName != lun1.VGName {
		t.Fatal("GetLUNByID not equals", string(b), string(b1))
	}

	lun2, err := GetLUNByLunID("lunStorageSystemId001", 1)
	b, _ = json.MarshalIndent(&lun, "", "  ")
	b2, _ := json.MarshalIndent(&lun2, "", "  ")
	if lun.CreatedAt.Format("2006-01-02 15:04:05") != lun2.CreatedAt.Format("2006-01-02 15:04:05") ||
		lun.HostLunID != lun2.HostLunID ||
		lun.ID != lun2.ID ||
		lun.Mappingto != lun2.Mappingto ||
		lun.Name != lun2.Name ||
		lun.RaidGroupID != lun2.RaidGroupID ||
		lun.SizeByte != lun2.SizeByte ||
		lun.StorageLunID != lun2.StorageLunID ||
		lun.StorageSystemID != lun2.StorageSystemID ||
		lun.VGName != lun2.VGName {
		t.Fatal("GetLUNByLunID not equals", string(b), string(b2))
	}

	hostLun := LUN{
		ID:              "lunId002",
		Name:            "lunName002",
		VGName:          "lunUnitId002_VG",
		RaidGroupID:     "lunRaidGroupId002",
		StorageSystemID: "lunStorageSystemId002",
		Mappingto:       "lunMapingto099",
		SizeByte:        2,
		HostLunID:       2,
		StorageLunID:    2,
		CreatedAt:       time.Now(),
	}
	err = insertLUN(t, hostLun)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := DelLUN(hostLun.ID)
		if err != nil {
			t.Fatal(err)
		}
	}()

	hostLunID, err := SelectHostLunIDByMapping("lunMapingto099")
	if err != nil {
		t.Fatal(err)
	}
	if len(hostLunID) != 2 {
		t.Fatal("SelectHostLunIDByMapping should be 2", hostLunID)
	}

	systemLun := LUN{
		ID:              "lunId003",
		Name:            "lunName003",
		VGName:          "lunUnitId003_VG",
		RaidGroupID:     "lunRaidGroupId003",
		StorageSystemID: "lunStorageSystemId002",
		Mappingto:       "lunMapingto003",
		SizeByte:        3,
		HostLunID:       3,
		StorageLunID:    3,
		CreatedAt:       time.Now(),
	}
	err = insertLUN(t, systemLun)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := DelLUN(systemLun.ID)
		if err != nil {
			t.Fatal(err)
		}
	}()

	lunIDBySystemID, err := SelectLunIDBySystemID("lunStorageSystemId002")
	if err != nil {
		t.Fatal(err)
	}
	if len(lunIDBySystemID) != 2 {
		t.Fatal("SelectLunIDBySystemID should be 2", lunIDBySystemID)
	}
}

func TestRaidGroup(t *testing.T) {
	rg := RaidGroup{
		ID:          "raidGroupId001",
		StorageID:   "raidGroupStorageID001",
		StorageRGID: 1,
		Enabled:     true,
	}
	err := rg.Insert()
	if err != nil {
		t.Fatal(err)
	}

	enabled := false
	rg.Enabled = enabled
	err = UpdateRaidGroupStatus(rg.StorageID, rg.StorageRGID, enabled)
	if err != nil {
		t.Fatal(err)
	}
	rg1, err := GetRaidGroup(rg.StorageID, rg.StorageRGID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(&rg)
	b1, _ := json.Marshal(&rg1)
	if !bytes.Equal(b, b1) {
		t.Fatal("UpdateRaidGroupStatus not equal", string(b), string(b1))
	}

	enabled = true
	rg.Enabled = enabled
	err = UpdateRaidGroupStatusByID(rg.ID, enabled)
	if err != nil {
		t.Fatal(err)
	}
	rg2, err := GetRaidGroup(rg.StorageID, rg.StorageRGID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(&rg)
	b2, _ := json.Marshal(&rg2)
	if !bytes.Equal(b, b2) {
		t.Fatal("UpdateRaidGroupStatusByID not equal", string(b), string(b2))
	}

	rg4 := RaidGroup{
		ID:          "raidGroupId002",
		StorageID:   rg.StorageID,
		StorageRGID: 2,
		Enabled:     rg.Enabled,
	}
	err = rg4.Insert()
	if err != nil {
		t.Fatal(err)
	}

	rg5, err := SelectRaidGroupByStorageID(rg.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rg5) != 2 {
		t.Fatal("SelectRaidGroupByStorageID should be 2", rg5)
	}
}

func TestHitachiStorage(t *testing.T) {
	hs := HitachiStorage{
		ID:        "HitachiStorageId001",
		Vendor:    "HitachiStorageVendor001",
		AdminUnit: "HitachiStorageAdminUnit001",
		LunStart:  1,
		LunEnd:    5,
		HluStart:  11,
		HluEnd:    55,
	}
	err := hs.Insert()
	if err != nil {
		t.Fatal(err)
	}
}

func TestHuaweiStorage(t *testing.T) {
	hs := HuaweiStorage{
		ID:       "HuaweiStorageID001",
		Vendor:   "HuaweiStorageVendor001",
		IPAddr:   "146.240.104.1",
		Username: "HuaweiStorageUsername001",
		Password: "HuaweiStoragePassword001",
		HluStart: 1,
		HluEnd:   5,
	}
	err := hs.Insert()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLocalVolume(t *testing.T) {
	lv1 := LocalVolume{
		ID:         "LocalVolumeID001",
		Name:       "LocalVolumeName001",
		Size:       1,
		VGName:     "LocalVolumeVGName001",
		Driver:     "LocalVolumeDriver001",
		Filesystem: "LocalVolumeFilesystem001",
	}
	defer func() {
		err := DeleteLocalVoume(lv1.ID)
		if err != nil {
			t.Fatal(err)
		}
	}()
	lv2 := LocalVolume{
		ID:         "LocalVolumeID002",
		Name:       "LocalVolumeName002",
		Size:       2,
		VGName:     "LocalVolumeVGName002",
		Driver:     "LocalVolumeDriver002",
		Filesystem: "LocalVolumeFilesystem002",
	}
	defer func() {
		err := DeleteLocalVoume(lv2.Name)
		if err != nil {
			t.Fatal(err)
		}
	}()
	lv3 := LocalVolume{
		ID:         "LocalVolumeID003",
		Name:       "LocalVolumeName003",
		Size:       3,
		VGName:     "LocalVolumeVGName002",
		Driver:     "LocalVolumeDriver003",
		Filesystem: "LocalVolumeFilesystem003",
	}
	defer func() {
		err := DeleteLocalVoume(lv3.Name)
		if err != nil {
			t.Fatal(err)
		}
	}()
	err := InsertLocalVolume(lv1)
	if err != nil {
		t.Fatal(err)
	}
	err = InsertLocalVolume(lv2)
	if err != nil {
		t.Fatal(err)
	}
	err = InsertLocalVolume(lv3)
	if err != nil {
		t.Fatal(err)
	}

	lv4, err := GetLocalVolume(lv1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lv4.ID == "" {
		t.Fatal("GetLocalVoume id should be 1", lv4)
	}

	lv5, err := GetLocalVolume(lv2.Name)
	if err != nil {
		t.Fatal(err)
	}
	if lv5.ID == "" {
		t.Fatal("GetLocalVoume name should be 1", lv5)
	}

	lv6, err := SelectVolumeByVG(lv3.VGName)
	if err != nil {
		t.Fatal(err)
	}
	if len(lv6) != 2 {
		t.Fatal("SelectVolumeByVG should be 2", lv6)
	}
}

func TestHisHuaStorage(t *testing.T) {
	hitachiStorage := HitachiStorage{
		ID:        "HitachiStorageId002",
		Vendor:    "HitachiStorageVendor002",
		AdminUnit: "HitachiStorageAdminUnit002",
		LunStart:  1,
		LunEnd:    5,
		HluStart:  11,
		HluEnd:    55,
	}
	err := hitachiStorage.Insert()
	if err != nil {
		t.Fatal(err)
	}

	huaweiStorage := HuaweiStorage{
		ID:       "HuaweiStorageID002",
		Vendor:   "HuaweiStorageVendor002",
		IPAddr:   "146.240.104.1",
		Username: "HuaweiStorageUsername002",
		Password: "HuaweiStoragePassword002",
		HluStart: 1,
		HluEnd:   5,
	}
	err = huaweiStorage.Insert()
	if err != nil {
		t.Fatal(err)
	}

	his1, hus1, err := GetStorageByID(hitachiStorage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if his1 == nil {
		t.Fatal("GetStorageByID his should get hitachiStorage")
	}
	if hus1 != nil {
		t.Fatal("GetStorageByID his should not get huaweiStorage")
	}

	his2, hus2, err := GetStorageByID(huaweiStorage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if his2 != nil {
		t.Fatal("GetStorageByID hus should not get hitachiStorage")
	}
	if hus2 == nil {
		t.Fatal("GetStorageByID hus should get huaweiStorage")
	}

	his3, hus3, err := GetStorageByID("99999")
	if err == nil {
		t.Fatal("GetStorageByID 99999 should err")
	}
	if his3 != nil {
		t.Fatal("GetStorageByID 99999 should not get hitachiStorage")
	}
	if hus3 != nil {
		t.Fatal("GetStorageByID 99999 should not get huaweiStorage")
	}
}
