package database

import (
	"encoding/json"
	"testing"
	"time"
)

func TestInsertTask(t *testing.T) {
	task1 := NewTask("TaskRelated001", "TaskLinkto001", "TaskDescription001", []string{"TaskLabels011", "TaskLabels011"}, 1011)
	err := task1.Insert()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTxInsertTask(t *testing.T) {
	task2 := NewTask("TaskRelated002", "TaskLinkto002", "TaskDescription002", []string{"TaskLabels021", "TaskLabels022"}, 1021)
	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxInsertTask(tx, task2)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTxInsertMultiTask(t *testing.T) {
	t1 := NewTask("TaskRelated003", "TaskLinkto003", "TaskDescription003", []string{"TaskLabels031", "TaskLabels031"}, 1031)
	t2 := NewTask("TaskRelated004", "TaskLinkto004", "TaskDescription004", []string{"TaskLabels041", "TaskLabels041"}, 1041)

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
}

func TestTxUpdateTaskStatusNotZero(t *testing.T) {
	task := Task{
		ID: "57b8db7218d32f615e8d48646d6007d3289ac7600fcb9eb0125905e42eacb8c0",
	}
	status := int64(1)
	finish := time.Now()
	msg := "msg001"

	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxUpdateTaskStatus(tx, &task, status, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTxUpdateTaskStatusZero(t *testing.T) {
	task := Task{
		ID: "a9cef18feb6b2a66539b2abf08be673490468d93fe3df04fe4d4300a5a97ff0c",
	}
	status := int64(2)
	finish := time.Time{}
	msg := "msg002"

	tx, err := GetTX()
	if err != nil {
		t.Fatal(err)
	}
	err = TxUpdateTaskStatus(tx, &task, status, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateTaskStatusNotZero(t *testing.T) {
	task := Task{
		ID: "5d3cb60881556a2d118459a43074775660a3f8a4f7b0999e59759e8af97c1aa8",
	}
	status := int64(1)
	finish := time.Now()
	msg := "msg001"
	err := UpdateTaskStatus(&task, status, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateTaskStatusZero(t *testing.T) {
	task := Task{
		ID: "5d62e9d5f15a71ab59c0db112d4339ee38ff38742790c74e46dbb89ca7c8fb51",
	}
	status := int64(2)
	finish := time.Time{}
	msg := "msg002"
	err := UpdateTaskStatus(&task, status, finish, msg)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryTask(t *testing.T) {
	id := "a9cef18feb6b2a66539b2abf08be673490468d93fe3df04fe4d4300a5a97ff0c"
	task, err := GetTask(id)
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("QueryTask should not be nil")
	}
	t.Log(task)
}

func TestDeleteTask(t *testing.T) {
	id := "a9cef18feb6b2a66539b2abf08be673490468d93fe3df04fe4d4300a5a97ff0c"
	err := DeleteTask(id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTxBackupTaskDone(t *testing.T) {
	task := Task{
		ID: "c3aa342ae7747addbd77789cc187be2cf7027d634a91d2abe57f4e5ccd05cc8d",
	}
	status := int64(1)

	bf := BackupFile{
		ID:         "BackupFileID001",
		TaskID:     "BackupFileTaskID001",
		StrategyID: "BackupFileStrategyID001",
		UnitID:     "BackupFileUnitID001",
		Type:       "BackupFileType001",
		Path:       "BackupFilePath001",
		SizeByte:   1,
		Retention:  time.Now(),
		CreatedAt:  time.Now(),
	}

	err := TxBackupTaskDone(&task, status, bf)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBackupStrategy(t *testing.T) {
	bs := BackupStrategy{
		ID:        "BackupStrategyID002",
		Type:      "BackupStrategyType002",
		ServiceID: "service0002",
		Spec:      "BackupStrategySpec002",
		Next:      time.Now(),
		Valid:     time.Now(),
		Enabled:   true,
		BackupDir: "BackupStrategyBackupDir002",
		Timeout:   1012,
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
	bs1, err := GetBackupStrategy(bs.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(&bs, "", "  ")
	b1, _ := json.MarshalIndent(bs1, "", "  ")
	if bs.BackupDir != bs1.BackupDir ||
		bs.CreatedAt.Format("2006-01-02 15:04:05") != bs1.CreatedAt.Format("2006-01-02 15:04:05") ||
		bs.Enabled != bs1.Enabled ||
		bs.ID != bs1.ID ||
		bs.Next.Format("2006-01-02 15:04:05") != bs1.Next.Format("2006-01-02 15:04:05") ||
		bs.Spec != bs1.Spec ||
		bs.Timeout != bs1.Timeout ||
		bs.Type != bs1.Type ||
		bs.Valid.Format("2006-01-02 15:04:05") != bs1.Valid.Format("2006-01-02 15:04:05") {
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
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(&bs, "", "  ")
	b2, _ := json.MarshalIndent(bs2, "", "  ")
	if bs.BackupDir != bs2.BackupDir ||
		bs.CreatedAt.Format("2006-01-02 15:04:05") != bs2.CreatedAt.Format("2006-01-02 15:04:05") ||
		bs.Enabled != bs2.Enabled ||
		bs.ID != bs2.ID ||
		bs.Next.Format("2006-01-02 15:04:05") != bs2.Next.Format("2006-01-02 15:04:05") ||
		bs.Spec != bs2.Spec ||
		bs.Timeout != bs2.Timeout ||
		bs.Type != bs2.Type ||
		bs.Valid.Format("2006-01-02 15:04:05") != bs2.Valid.Format("2006-01-02 15:04:05") {
		t.Fatal("UpdateNext should be equal", b, b2)
	}
}

func TestBackupTaskValidate(t *testing.T) {
	taskID := "1aa46006cb43690997af5e02231ec4b3081dcc611849d7051d4e62aac0930ba6"
	strategyID := "BackupStrategyID001"
	unitID := ""
	task, retention, err := BackupTaskValidate(taskID, strategyID, unitID)
	if err == nil {
		t.Fatal("Error Expected")
	}

	t.Log(err, task)

	if retention == 0 {
		t.Fatal("BackupTaskValidate retention should not be 0", retention)
	}
}
