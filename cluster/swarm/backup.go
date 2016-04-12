package swarm

import (
	"time"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	"github.com/yiduoyunQ/smlib"
	crontab "gopkg.in/robfig/cron.v2"
)

type serviceBackup struct {
	id       crontab.EntryID
	server   string
	strategy *database.BackupStrategy
	schedule crontab.Schedule

	svc *Service
}

func NewBackupJob(addr string, svc *Service) *serviceBackup {
	return &serviceBackup{
		svc:      svc,
		server:   addr,
		strategy: svc.backup,
	}
}

func (bs *serviceBackup) Run() {
	strategy, err := database.GetBackupStrategy(bs.strategy.ID)
	if err != nil {
		return
	}

	if !strategy.Enabled || bs.server == "" {
		return
	}

	task := database.NewTask("backup_strategy", strategy.ID, "", nil, strategy.Timeout)
	task.Status = _StatusTaskCreate

	err = database.InsertTask(task)
	if err != nil {
		return
	}

	bs.strategy = strategy

	addr, port, master, err := bs.svc.GetSwitchManagerAndMaster()
	if err != nil {
		err = database.UpdateTaskStatus(task, _StatusTaskCancel, time.Now(), "Cancel,The Task marked as TaskCancel,"+err.Error())

		return
	}

	if err := smlib.Lock(addr, port); err != nil {
		err = database.UpdateTaskStatus(task, _StatusTaskCancel, time.Now(), "TaskCancel,Switch Manager is busy now,"+err.Error())

		return
	}
	defer smlib.UnLock(addr, port)

	args := []string{bs.server + "v1.0/task/backup/callback", task.ID, strategy.ID, master.ID, strategy.Type}

	errCh := make(chan error, 1)
	select {
	case errCh <- master.backup(args...):

	case <-time.After(strategy.Timeout):
		err = database.UpdateTaskStatus(task, _StatusTaskTimeout, time.Now(), "Timeout,The Task marked as TaskTimeout")
	}

	<-errCh

	return

}

func (bs *serviceBackup) Next(time.Time) time.Time {
	next := time.Time{}

	if bs.strategy == nil {
		return next
	}

	strategy, err := database.GetBackupStrategy(bs.strategy.ID)
	if err != nil {
		return next
	}

	if bs.schedule == nil {
		bs.schedule, err = crontab.Parse(bs.strategy.Spec)
		if err != nil {
			return next
		}
	}

	if strategy.Enabled {
		next = bs.schedule.Next(time.Now())
	}

	if next.IsZero() || next.After(strategy.Valid) {
		strategy.UpdateNext(next, false)
		bs.strategy = strategy

		return time.Time{}
	}

	err = strategy.UpdateNext(next, true)
	if err != nil {
		return time.Time{}
	}

	bs.strategy = strategy

	return next
}

func (gd *Gardener) RegisterBackupStrategy(strategy *serviceBackup) error {
	gd.Lock()

	for key, val := range gd.cronJobs {
		if val.strategy.ID == strategy.strategy.ID &&
			val.svc.ID == strategy.svc.ID {

			entry := gd.cron.Entry(key)
			if entry.ID == key {
				// already exist
				gd.Unlock()

				return nil
			}
		}
	}

	id := gd.cron.Schedule(strategy, strategy)
	strategy.id = id

	gd.cronJobs[id] = strategy
	gd.Unlock()

	return nil
}

func (gd *Gardener) RemoveCronJob(strategyID string) error {
	gd.Lock()

	for key, val := range gd.cronJobs {
		if val.strategy.ID == strategyID {
			gd.cron.Remove(key)
			delete(gd.cronJobs, key)

		}
	}

	gd.Unlock()

	return nil
}

func BackupTaskCallback(req structs.BackupTaskCallback) error {
	task := &database.Task{ID: req.TaskID}

	if req.Error() != nil {
		err := database.UpdateTaskStatus(task, structs.TaskFailed, time.Now(), req.Error().Error())
		if err != nil {
			return err
		}

		return req.Error()
	}

	rent, err := database.BackupTaskValidate(req.TaskID, req.StrategyID, req.UnitID)
	if err != nil {

		return err
	}

	backupFile := database.BackupFile{
		ID:         utils.Generate64UUID(),
		TaskID:     req.TaskID,
		StrategyID: req.StrategyID,
		UnitID:     req.UnitID,
		Type:       req.Type,
		Path:       req.Path,
		SizeByte:   req.Size,
		CreatedAt:  time.Now(),
	}

	if rent > 0 {
		backupFile.Retention = backupFile.CreatedAt.Add(time.Duration(rent))
	}

	err = database.TxBackupTaskDone(task, _StatusTaskDone, backupFile)

	return err
}
