package swarm

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
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

	if !strategy.Enabled || bs.server == "" || bs.svc == nil {
		return
	}
	bs.strategy = strategy

	task := database.NewTask("backup_strategy", strategy.ID, "", nil, strategy.Timeout)
	task.Status = _StatusTaskCreate
	err = database.InsertTask(task)
	if err != nil {
		return
	}

	bs.svc.TryBackupTask(&task, bs.server, "", *strategy)
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

	next = bs.schedule.Next(time.Now())

	if next.IsZero() || next.After(strategy.Valid) {
		strategy.UpdateNext(next, false)
		bs.strategy = strategy

		return time.Time{}
	}

	err = strategy.UpdateNext(next, true)
	if err != nil {
		return next
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

func (gd *Gardener) ReplaceServiceBackupStrategy(NameOrID string, req structs.BackupStrategy) (*database.BackupStrategy, error) {
	service, err := gd.GetService(NameOrID)
	if err != nil {
		return nil, err
	}

	strategy, err := service.ReplaceBackupStrategy(req)
	if err != nil {
		return strategy, err
	}

	if service.backup != nil {
		bs := NewBackupJob(gd.host, service)
		err = gd.RegisterBackupStrategy(bs)
		if err != nil {
			logrus.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err.Error())
		}
	}

	return strategy, err
}

func (gd *Gardener) UpdateServiceBackupStrategy(NameOrID string, req structs.BackupStrategy) error {
	var (
		valid = time.Time{}
		err   error
	)
	if req.Valid != "" {
		valid, err = utils.ParseStringToTime(req.Valid)
		if err != nil {
			logrus.Error("Parse Request.BackupStrategy.Valid to time.Time", err)
			return err
		}
	}
	bs, err := database.GetBackupStrategy(NameOrID)
	if err != nil {
		return err
	}

	update := database.BackupStrategy{
		ID:        bs.ID,
		ServiceID: bs.ServiceID,
		Name:      req.Name,
		Type:      req.Type,
		Spec:      req.Spec,
		Valid:     valid,
		BackupDir: req.BackupDir,
		Timeout:   req.Timeout * int(time.Second),
		Next:      bs.Next,
		Enabled:   req.Enable,
		CreatedAt: bs.CreatedAt,
	}

	err = database.UpdateBackupStrategy(update)

	return err
}

func (gd *Gardener) EnableServiceBackupStrategy(strategy string) error {
	backup, err := database.GetBackupStrategy(strategy)
	if err != nil || backup == nil {
		return fmt.Errorf("Not Found BackupStrategy,%v", err)
	}

	err = database.UpdateBackupStrategyStatus(strategy, true)
	if err != nil {
		return err
	}
	backup.Enabled = true

	svc, err := gd.GetService(backup.ServiceID)
	if err == nil && svc != nil {
		svc.Lock()
		svc.backup = backup
		svc.Unlock()

		bs := NewBackupJob(gd.host, svc)
		err = gd.RegisterBackupStrategy(bs)
		if err != nil {
			logrus.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err)
		}

		return err
	}

	return err
}

func (gd *Gardener) DisableBackupStrategy(id string) error {

	return database.UpdateBackupStrategyStatus(id, false)
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
		backupFile.Retention = backupFile.CreatedAt.AddDate(0, 0, rent)
	}

	err = database.TxBackupTaskDone(task, _StatusTaskDone, backupFile)

	return err
}
