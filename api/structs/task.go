package structs

import (
	"fmt"
	"time"
)

const (
	TaskCreate = iota
	TaskRunning
	TaskStop
	TaskCancel
	TaskDone
	TaskTimeout
	TaskFailed
)

type TaskResponse struct {
	ID string `json:"id"`
	// Name     string `json:"name"`
	Related     string `json:"related"`
	Linkto      string `json:"link_to"`
	Description string `json:"description"`
	Labels      string `json:"labels"`
	Errors      string `json:"errors"`
	Timeout     int    `json:"timeout"` // s
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at"`
	FinishedAt  string `json:"finished_at"`
}

type BackupTaskCallback struct {
	TaskID     string `json:"task_id"`
	StrategyID string `json:"strategy_id"`
	UnitID     string `json:"unit_id"`
	Type       string `json:"type,omitempty"`
	Path       string `json:"path,omitempty"`
	Code       byte   `json:"code"`
	Size       int    `json:"size,omitempty"`
	Msg        string `json:"msg,omitempty"`
}

func (bt BackupTaskCallback) Error() error {
	if bt.Code == 0 {
		return nil
	}

	return fmt.Errorf("Backup Task Error:%s,unitID:%s ,taskID:%s ,strategyID:%s code:%d",
		bt.Msg, bt.UnitID, bt.TaskID, bt.StrategyID, bt.Code)
}

type BackupFile struct {
	ID         string
	Name       string
	TaskID     string `json:"task_id"`
	StrategyID string `json:"strategy_id"`
	UnitID     string `json:"unit_id"`
	Type       string // full or incremental
	Path       string
	SizeByte   int
	Retention  string
	CreatedAt  string    `json:"created_at"`
	FinishedAt string    `json:"finished_at"`
	Created    time.Time `json:"-"`
}

type BackupFiles []BackupFile

func (bfs BackupFiles) Len() int { return len(bfs) }
func (bfs BackupFiles) Less(i, j int) bool {
	return bfs[i].Created.After(bfs[j].Created)
}
func (bfs BackupFiles) Swap(i, j int) {
	bfs[i], bfs[j] = bfs[j], bfs[i]
}
