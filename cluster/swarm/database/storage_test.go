package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/docker/swarm/utils"
)

func insertLUN(t *testing.T, lun LUN) error {
	db, err := getDB(true)
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
		ID:              utils.Generate64UUID(),
		Name:            utils.Generate64UUID(),
		VGName:          "lunName001_VG",
		RaidGroupID:     "lunRaidGroupId001",
		StorageSystemID: "lunStorageSystemId001",
		MappingTo:       "lunMapingto001",
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
	lun.MappingTo = host
	lun.VGName = ""
	lun.HostLunID = hlun
	err = LunMapping(lun.ID, host, vgName, hlun)
	if err != nil {
		t.Fatal(err)
	}

	lun1, err := GetLUNByID(lun.ID)
	b, _ := json.MarshalIndent(&lun, "", "  ")
	b1, _ := json.MarshalIndent(&lun1, "", "  ")
	if lun.HostLunID != lun1.HostLunID ||
		lun.ID != lun1.ID ||
		lun.MappingTo != lun1.MappingTo ||
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
	if lun.HostLunID != lun2.HostLunID ||
		lun.ID != lun2.ID ||
		lun.MappingTo != lun2.MappingTo ||
		lun.Name != lun2.Name ||
		lun.RaidGroupID != lun2.RaidGroupID ||
		lun.SizeByte != lun2.SizeByte ||
		lun.StorageLunID != lun2.StorageLunID ||
		lun.StorageSystemID != lun2.StorageSystemID ||
		lun.VGName != lun2.VGName {
		t.Fatal("GetLUNByLunID not equals", string(b), string(b2))
	}

	hostLun := LUN{
		ID:              utils.Generate64UUID(),
		Name:            utils.Generate64UUID(),
		VGName:          "lunUnitId002_VG",
		RaidGroupID:     "lunRaidGroupId002",
		StorageSystemID: "lunStorageSystemId002",
		MappingTo:       "lunMapingto099",
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

	hostLunID, err := ListHostLunIDByMapping("lunMapingto099")
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
		MappingTo:       "lunMapingto003",
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

	lunIDBySystemID, err := ListLunIDBySystemID("lunStorageSystemId002")
	if err != nil {
		t.Fatal(err)
	}
	if len(lunIDBySystemID) != 2 {
		t.Fatal("SelectLunIDBySystemID should be 2", lunIDBySystemID)
	}
}

func TestRaidGroup(t *testing.T) {
	rg := RaidGroup{
		ID:          utils.Generate64UUID(),
		StorageID:   "raidGroupStorageID001",
		StorageRGID: 1,
		Enabled:     true,
	}
	err := rg.Insert()
	if err != nil {
		t.Fatal(err)
	}

	defer DeleteRaidGroup(rg.StorageID, rg.StorageRGID)

	err = UpdateRaidGroupStatus(rg.StorageID, rg.StorageRGID, false)
	if err != nil {
		t.Fatal(err)
	}
	rg1, err := GetRaidGroup(rg.StorageID, rg.StorageRGID)
	if err != nil {
		t.Fatal(err, rg1)
	}

	err = UpdateRGStatusByID(rg.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	rg2, err := GetRaidGroup(rg.StorageID, rg.StorageRGID)
	if err != nil {
		t.Fatal(err, rg2)
	}

	rg4 := RaidGroup{
		ID:          utils.Generate64UUID(),
		StorageID:   rg.StorageID,
		StorageRGID: 2,
		Enabled:     rg.Enabled,
	}
	err = rg4.Insert()
	if err != nil {
		t.Fatal(err)
	}

	defer DeleteRaidGroup(rg4.StorageID, rg4.StorageRGID)

	rg5, err := ListRGByStorageID(rg.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rg5) != 2 {
		t.Fatal("SelectRaidGroupByStorageID should be 2", rg5)
	}
}

func TestHitachiStorage(t *testing.T) {
	hs := HitachiStorage{
		ID:        utils.Generate64UUID(),
		Vendor:    "HitachiStorageVendor001",
		AdminUnit: utils.Generate64UUID(),
		LunStart:  1,
		LunEnd:    5,
		HluStart:  11,
		HluEnd:    55,
	}
	err := hs.Insert()
	if err != nil {
		t.Fatal(err)
	}

	err = DeleteStorageByID(hs.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHuaweiStorage(t *testing.T) {
	hs := HuaweiStorage{
		ID:       utils.Generate64UUID(),
		Vendor:   "HuaweiStorageVendor001",
		IPAddr:   randomIP(),
		Username: "HuaweiStorageUsername001",
		Password: "HuaweiStoragePassword001",
		HluStart: 1,
		HluEnd:   5,
	}
	err := hs.Insert()
	if err != nil {
		t.Fatal(err)
	}

	err = DeleteStorageByID(hs.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLocalVolume(t *testing.T) {
	lv1 := LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       utils.Generate64UUID(),
		Size:       1,
		VGName:     "LocalVolumeVGName001",
		Driver:     "LocalVolumeDriver001",
		Filesystem: "LocalVolumeFilesystem001",
	}

	lv2 := LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       utils.Generate64UUID(),
		Size:       2,
		VGName:     "LocalVolumeVGName002",
		Driver:     "LocalVolumeDriver002",
		Filesystem: "LocalVolumeFilesystem002",
	}

	lv3 := LocalVolume{
		ID:         utils.Generate64UUID(),
		Name:       "LocalVolumeName003",
		Size:       3,
		VGName:     "LocalVolumeVGName002",
		Driver:     "LocalVolumeDriver003",
		Filesystem: "LocalVolumeFilesystem003",
	}

	defer func() {
		err := DeleteLocalVoume(lv1.Name)
		if err != nil {
			t.Error(err)
		}
		err = DeleteLocalVoume(lv2.Name)
		if err != nil {
			t.Error(err)
		}
		err = DeleteLocalVoume(lv3.Name)
		if err != nil {
			t.Error(err)
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

	lv6, err := ListVolumeByVG(lv3.VGName)
	if err != nil {
		t.Fatal(err)
	}
	if len(lv6) != 2 {
		t.Fatal("SelectVolumeByVG should be 2", lv6)
	}
}

func TestGetStorageByID(t *testing.T) {
	hitachiStorage := HitachiStorage{
		ID:        utils.Generate64UUID(),
		Vendor:    "HitachiStorageVendor002",
		AdminUnit: utils.Generate64UUID(),
		LunStart:  1,
		LunEnd:    5,
		HluStart:  11,
		HluEnd:    55,
	}
	huaweiStorage := HuaweiStorage{
		ID:       utils.Generate64UUID(),
		Vendor:   "HuaweiStorageVendor002",
		IPAddr:   randomIP(),
		Username: "HuaweiStorageUsername002",
		Password: "HuaweiStoragePassword002",
		HluStart: 1,
		HluEnd:   5,
	}

	err := hitachiStorage.Insert()
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteStorageByID(hitachiStorage.ID)

	err = huaweiStorage.Insert()
	if err != nil {
		t.Fatal(err)
	}
	defer DeleteStorageByID(huaweiStorage.ID)

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
