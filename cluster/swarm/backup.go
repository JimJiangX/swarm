package swarm

import (
	"sync"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
	crontab "gopkg.in/robfig/cron.v2"
)

type serviceBackup struct {
	id       crontab.EntryID
	lock     *sync.Mutex
	strategy *database.BackupStrategy
	schedule crontab.Schedule

	svc *Service
}

func NewBackupJob(svc *Service) *serviceBackup {
	return &serviceBackup{
		svc:      svc,
		lock:     new(sync.Mutex),
		strategy: svc.backup,
	}
}

func (backup *serviceBackup) Run() {
	backup.lock.Lock()
	defer backup.lock.Unlock()

}

func (backup *serviceBackup) Next(time.Time) time.Time {
	backup.lock.Lock()
	defer backup.lock.Unlock()

	next := time.Time{}
	if backup.strategy == nil {
		return next
	}

	strategy, err := database.GetBackupStrategy(backup.strategy.ID)
	if err != nil {
		return next
	}

	now := time.Now()

	if !strategy.Enabled || now.After(strategy.Valid) {
		backup.strategy.UpdateNext(next, false, 2)
		return next
	}

	backup.strategy = strategy

	if backup.schedule == nil {
		backup.schedule, err = crontab.Parse(backup.strategy.Spec)
		if err != nil {
			return next
		}
	}

	next = backup.schedule.Next(now)

	err = backup.strategy.UpdateNext(next, true, 1)
	if err != nil {

	}

	return next

}
