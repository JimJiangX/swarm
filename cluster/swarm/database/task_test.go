package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/docker/swarm/utils"
)

func TestInsertTask(t *testing.T) {
	task := NewTask(utils.Generate64UUID(), "TaskRelated001", "TaskLinkto001", "TaskDescription001", []string{"TaskLabels011", "TaskLabels011"}, 1011)
	err := task.Insert()
	if err != nil {
		t.Fatal(err)
	}

	err = DeleteTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTxInsertTask(t *testing.T) {
	task := NewTask(utils.Generate64UUID(), "TaskRelated002", "TaskLinkto002", "TaskDescription002", []string{"TaskLabels021", "TaskLabels022"}, 1021)
	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxInsertTask(tx, task)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	err = DeleteTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTxInsertMultiTask(t *testing.T) {
	t1 := NewTask(utils.Generate64UUID(), "TaskRelated003", "TaskLinkto003", "TaskDescription003", []string{"TaskLabels031", "TaskLabels031"}, 1031)
	t2 := NewTask(utils.Generate64UUID(), "TaskRelated004", "TaskLinkto004", "TaskDescription004", []string{"TaskLabels041", "TaskLabels041"}, 1041)

	tasks := []*Task{&t1, &t2}
	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxInsertMultiTask(tx, tasks)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	defer DeleteTask(t1.ID)
	defer DeleteTask(t2.ID)

	task, err := GetTask(t1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task == (Task{}) {
		t.Fatal("QueryTask should not be nil")
	}

	status := int64(1)
	finish := time.Now()
	msg := "msg001"

	tx, err = GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = txUpdateTaskStatus(tx, &t1, status, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
	err = txUpdateTaskStatus(tx, &t2, status, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	status = int64(3)
	finish = time.Now()
	msg = "msg001000"
	err = UpdateTaskStatus(&t1, status, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
}

func deleteBackupFile(ID string) error {
	db, err := GetDB(false)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tbl_dbaas_backup_files WHERE id=?", ID)

	return err
}

func TestTxBackupTaskDone(t *testing.T) {
	task := NewTask(utils.Generate64UUID(), "TaskRelated001", "TaskLinkto001", "TaskDescription001", []string{"TaskLabels011", "TaskLabels011"}, 1011)
	err := task.Insert()
	if err != nil {
		t.Fatal(err)
	}

	defer DeleteTask(task.ID)

	status := int64(100)

	bf := BackupFile{
		ID:         utils.Generate64UUID(),
		TaskID:     "BackupFileTaskID001",
		StrategyID: "BackupFileStrategyID001",
		UnitID:     "BackupFileUnitID001",
		Type:       "BackupFileType001",
		Path:       "BackupFilePath001",
		SizeByte:   1,
		Retention:  time.Now(),
		CreatedAt:  time.Now(),
	}

	err = TxBackupTaskDone(&task, status, bf)
	if err != nil {
		t.Fatal(err)
	}

	err = deleteBackupFile(bf.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBackupStrategy(t *testing.T) {
	bs := BackupStrategy{
		ID:        utils.Generate64UUID(),
		Name:      utils.Generate64UUID(),
		Type:      "BackupStrategyType002",
		ServiceID: "service0002",
		Spec:      "BackupStrategySpec002",
		Next:      time.Now(),
		Valid:     time.Now(),
		Enabled:   true,
		BackupDir: "BackupStrategyBackupDir002",
		Timeout:   100000,
		CreatedAt: time.Now(),
	}

	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxInsertBackupStrategy(tx, bs)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}

	defer DeleteBackupStrategy(bs.ID)

	bs1, err := GetBackupStrategy(bs.ID)
	if err != nil {
		t.Skip(err)
	}
	b, _ := json.MarshalIndent(&bs, "", "  ")
	b1, _ := json.MarshalIndent(bs1, "", "  ")
	if bs.BackupDir != bs1.BackupDir ||
		bs.Enabled != bs1.Enabled ||
		bs.ID != bs1.ID ||
		bs.Spec != bs1.Spec ||
		bs.Timeout != bs1.Timeout ||
		bs.Type != bs1.Type {
		t.Fatal("GetBackupStrategy should be equal", string(b), string(b1))
	}

	time.Sleep(time.Second)
	next := time.Now()
	enable := false
	bs.Next = next
	bs.Enabled = enable
	err = bs.UpdateNext(next, enable)
	if err != nil {
		t.Fatal(err)
	}
	bs2, err := GetBackupStrategy(bs.ID)
	if err != nil {
		t.Skip(err)
	}
	b, _ = json.MarshalIndent(&bs, "", "  ")
	b2, _ := json.MarshalIndent(bs2, "", "  ")
	if bs.BackupDir != bs2.BackupDir ||
		bs.Enabled != bs2.Enabled ||
		bs.ID != bs2.ID ||
		bs.Spec != bs2.Spec ||
		bs.Timeout != bs2.Timeout ||
		bs.Type != bs2.Type {
		t.Fatal("UpdateNext should be equal", b, b2)
	}
}

func TestBackupTaskValidate(t *testing.T) {
	taskID := "1aa46006cb43690997af5e02231ec4b3081dcc611849d7051d4e62aac0930ba6"
	strategyID := "BackupStrategyID001"
	unitID := ""
	_, _, err := BackupTaskValidate(taskID, strategyID, unitID)
	if err == nil {
		t.Fatal("Error Expected")
	}
}
