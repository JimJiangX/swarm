package database

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUnit(t *testing.T) {
	unit := Unit{
		ID:          "unit001",
		Name:        "unitName001",
		Type:        "upsql",
		ImageID:     "imageId001",
		ImageName:   "imageName001",
		ServiceID:   "serviceId001",
		EngineID:    "engineID001",
		ContainerID: "containerId001",
		ConfigID:    "configId001",
		NetworkMode: "networkMode001",

		Status:        1,
		CheckInterval: 1,
		CreatedAt:     time.Now(),
	}

	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxInsertUnit(tx, unit)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	unit1 := &Unit{
		ID:          "unit002",
		Name:        "unitName002",
		Type:        "upproxy",
		ImageID:     "imageId002",
		ImageName:   "imageName002",
		ServiceID:   "serviceId002",
		EngineID:    "engineID002",
		ContainerID: "containerId002",
		ConfigID:    "configId002",
		NetworkMode: "networkMode002",

		Status:        2,
		CheckInterval: 2,
		CreatedAt:     time.Now(),
	}
	unit2 := &Unit{
		ID:          "unit003",
		Name:        "unitName003",
		Type:        "switch_manager",
		ImageID:     "imageId003",
		ImageName:   "imageName003",
		ServiceID:   "serviceId003",
		EngineID:    "engineID003",
		ContainerID: "containerId003",
		ConfigID:    "configId003",
		NetworkMode: "networkMode003",

		Status:        3,
		CheckInterval: 3,
		CreatedAt:     time.Now(),
	}
	units := []*Unit{
		unit1,
		unit2,
	}

	tx, err = GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxInsertMultiUnit(tx, units)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	tx, err = GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxDeleteUnit(tx, unit2.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	unitConfig := UnitConfig{
		ID:        "unitConfig99",
		ImageID:   "imageId99",
		Mount:     "/tmp",
		Version:   99,
		ParentID:  "parentId99",
		Content:   "content99",
		KeySets:   make(map[string]KeysetParams),
		CreatedAt: time.Now(),
	}

	err = SaveUnitConfig(unit1, unitConfig)
	if err != nil {
		t.Fatal(err)
	}
}

func TestService(t *testing.T) {
	service := Service{
		ID:                   "serviceId001",
		Name:                 "serviceName001",
		Desc:                 "serviceDescription001",
		Architecture:         "serviceArchitecture001",
		AutoHealing:          true,
		AutoScaling:          true,
		HighAvailable:        true,
		Status:               1,
		BackupMaxSizeByte:    79294802,
		BackupFilesRetention: 3258011085015,
		CreatedAt:            time.Now(),
		FinishedAt:           time.Now(),
	}
	backupStrategy := BackupStrategy{
		ID:        "backupStrategyId001",
		Type:      "backupStrategyType001",
		ServiceID: service.ID,
		Spec:      "backupStrategySpec001",
		Next:      time.Now(),
		Valid:     time.Now(),
		Enabled:   true,
		BackupDir: "backupStrategyBackupDir001",
		Timeout:   1012,
		CreatedAt: time.Now(),
	}
	task := Task{
		ID: "taskId001",
		//	Name        string        `db:"name"`
		Related:     "taskRelated001",
		Linkto:      "taskLinkto001",
		Description: "taskDescription001",
		Labels:      "taskLabels001",
		Errors:      "taskErrors001",
		Timeout:     1011,
		Status:      1,
		CreatedAt:   time.Now(),
		FinishedAt:  time.Now(),
	}

	user1 := User{
		ID:        "userId001",
		ServiceID: "userServiceId001",
		Type:      "userType001",
		Username:  "userName001",
		Password:  "userPassword001",
		Role:      "userRole001",
		CreatedAt: time.Now(),
	}
	user2 := User{
		ID:        "userId002",
		ServiceID: "userServiceId002",
		Type:      "userType002",
		Username:  "userName002",
		Password:  "userPassword002",
		Role:      "userRole002",
		CreatedAt: time.Now(),
	}
	var users []User
	users = append(users, user1)
	users = append(users, user2)

	err := TxSaveService(service, &backupStrategy, &task, users)
	if err != nil {
		t.Fatal(err)
	}

	service1, err := GetService(service.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(&service, "", "  ")
	b1, _ := json.MarshalIndent(&service1, "", "  ")
	if service.Architecture != service1.Architecture ||
		service.AutoHealing != service1.AutoHealing ||
		service.AutoScaling != service1.AutoScaling ||
		service.BackupFilesRetention != service1.BackupFilesRetention ||
		service.BackupMaxSizeByte != service1.BackupMaxSizeByte ||
		service.CreatedAt.Format("2006-01-02 15:04:05") != service1.CreatedAt.Format("2006-01-02 15:04:05") ||
		service.Desc != service1.Desc ||
		service.FinishedAt.Format("2006-01-02 15:04:05") != service1.FinishedAt.Format("2006-01-02 15:04:05") ||
		service.HighAvailable != service1.HighAvailable {
		t.Fatal("GetService not equal", string(b), string(b1))
	}

	status := int64(2)
	finish := time.Now()

	err = service.SetServiceStatus(status, finish)
	if err != nil {
		t.Fatal(err)
	}

	service.Status = status
	service.FinishedAt = finish

	service2, err := GetService(service.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(&service, "", "  ")
	b2, _ := json.MarshalIndent(&service2, "", "  ")
	if service.Architecture != service2.Architecture ||
		service.AutoHealing != service2.AutoHealing ||
		service.AutoScaling != service2.AutoScaling ||
		service.BackupFilesRetention != service2.BackupFilesRetention ||
		service.BackupMaxSizeByte != service2.BackupMaxSizeByte ||
		service.CreatedAt.Format("2006-01-02 15:04:05") != service2.CreatedAt.Format("2006-01-02 15:04:05") ||
		service.Desc != service2.Desc ||
		service.FinishedAt.Format("2006-01-02 15:04:05") != service2.FinishedAt.Format("2006-01-02 15:04:05") ||
		service.HighAvailable != service2.HighAvailable {
		t.Fatal("SetServiceStatus not equal", string(b), string(b2))
	}

	status = int64(3)
	tStatus := int64(4)
	finish = time.Now()
	msg := "msg001"
	service.Status = status
	service.FinishedAt = finish
	task.Status = tStatus
	task.FinishedAt = finish
	task.Errors = msg
	err = TxSetServiceStatus(&service, &task, status, tStatus, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
	service3, err := GetService(service.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(&service, "", "  ")
	b3, _ := json.MarshalIndent(&service3, "", "  ")
	if service.Architecture != service3.Architecture ||
		service.AutoHealing != service3.AutoHealing ||
		service.AutoScaling != service3.AutoScaling ||
		service.BackupFilesRetention != service3.BackupFilesRetention ||
		service.BackupMaxSizeByte != service3.BackupMaxSizeByte ||
		service.CreatedAt.Format("2006-01-02 15:04:05") != service3.CreatedAt.Format("2006-01-02 15:04:05") ||
		service.Desc != service3.Desc ||
		service.FinishedAt.Format("2006-01-02 15:04:05") != service3.FinishedAt.Format("2006-01-02 15:04:05") ||
		service.HighAvailable != service3.HighAvailable {
		t.Fatal("TxSetServiceStatus not equal", string(b), string(b3))
	}
	task1, err := GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(&task, "", "  ")
	b4, _ := json.MarshalIndent(task1, "", "  ")
	if task.CreatedAt.Format("2006-01-02 15:04:05") != task1.CreatedAt.Format("2006-01-02 15:04:05") ||
		task.Description != task1.Description ||
		task.Errors != task1.Errors ||
		task.FinishedAt.Format("2006-01-02 15:04:05") != task1.FinishedAt.Format("2006-01-02 15:04:05") ||
		task.Labels != task1.Labels ||
		task.Linkto != task1.Linkto ||
		task.Related != task1.Related ||
		task.Status != task1.Status ||
		task.Timeout != task1.Timeout {
		t.Fatal("TxSetServiceStatus not equal", string(b), string(b4))
	}

	finish = time.Time{}
	err = TxSetServiceStatus(&service, &task, status, tStatus, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
	service4, err := GetService(service.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(&service, "", "  ")
	b5, _ := json.MarshalIndent(&service4, "", "  ")
	if service.Architecture != service4.Architecture ||
		service.AutoHealing != service4.AutoHealing ||
		service.AutoScaling != service4.AutoScaling ||
		service.BackupFilesRetention != service4.BackupFilesRetention ||
		service.BackupMaxSizeByte != service4.BackupMaxSizeByte ||
		service.CreatedAt.Format("2006-01-02 15:04:05") != service4.CreatedAt.Format("2006-01-02 15:04:05") ||
		service.Desc != service4.Desc ||
		service.FinishedAt.Format("2006-01-02 15:04:05") != service4.FinishedAt.Format("2006-01-02 15:04:05") ||
		service.HighAvailable != service4.HighAvailable {
		t.Fatal("TxSetServiceStatus not equal", string(b), string(b5))
	}
	task2, err := GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(&task, "", "  ")
	b6, _ := json.MarshalIndent(task2, "", "  ")
	if task.CreatedAt.Format("2006-01-02 15:04:05") != task1.CreatedAt.Format("2006-01-02 15:04:05") ||
		task.Description != task2.Description ||
		task.Errors != task2.Errors ||
		task.FinishedAt.Format("2006-01-02 15:04:05") != task2.FinishedAt.Format("2006-01-02 15:04:05") ||
		task.Labels != task2.Labels ||
		task.Linkto != task2.Linkto ||
		task.Related != task2.Related ||
		task.Status != task2.Status ||
		task.Timeout != task2.Timeout {
		t.Fatal("TxSetServiceStatus not equal", string(b), string(b6))
	}

}
