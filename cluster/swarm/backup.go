package swarm

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
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

	bs.svc.TryBackupTask(bs.server, "", strategy.ID, strategy.Type, strategy.Timeout)
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

	if !strategy.Enabled || next.IsZero() || next.After(strategy.Valid) {
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

func (gd *Gardener) EnableServiceBackupStrategy(NameOrID, strategy string) error {
	backup, err := database.GetBackupStrategy(strategy)
	if err != nil {
		return fmt.Errorf("Not Found BackupStrategy,%s", err.Error())
	}

	svc, err := gd.GetService(NameOrID)
	if err == nil && svc != nil {
		if svc.backup.ID == backup.ID {
			err := database.UpdateBackupStrategyStatus(backup.ID, true)
			if err != nil {
				return err
			}
			svc.backup.Enabled = true

			return nil
		} else {
			err := database.TxUpdateServiceBackupStrategy(NameOrID, svc.BackupStrategyID, backup.ID)
			if err != nil {
				return err
			}
			svc.backup.Enabled = false
			backup.Enabled = true
			svc.backup = backup

			bs := NewBackupJob(gd.host, svc)
			err = gd.RegisterBackupStrategy(bs)
			if err != nil {
				log.Errorf("Add BackupStrategy to Gardener.Crontab Error:%s", err.Error())
			}

			return err
		}
	}
	// when not found service in Gardener

	return nil
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
		backupFile.Retention = backupFile.CreatedAt.Add(time.Duration(rent))
	}

	err = database.TxBackupTaskDone(task, _StatusTaskDone, backupFile)

	return err
}
