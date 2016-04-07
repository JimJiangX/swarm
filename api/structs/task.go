package structs

import (
	"fmt"
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

type BackupTaskCallback struct {
	TaskID     string `json:"task_id"`
	StrategyID string `json:"strategy_id"`
	UnitID     string `json:"unit_id"`
	Type       string `json:"type,omitempty"`
	Path       string `json:"path,omitempty"`
	Status     byte   `json:"status"`
	Size       int    `json:"size,omitempty"`
	Msg        string `json:"msg,omitempty"`
}

func (bt BackupTaskCallback) Error() error {
	if bt.Status == 0 {
		return nil
	}

	return fmt.Errorf("Backup Task Error:%s,unitID:%s ,taskID:%s ,strategyID:%s status:%d",
		bt.Msg, bt.UnitID, bt.TaskID, bt.StrategyID, bt.Status)
}
