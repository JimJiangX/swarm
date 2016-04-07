package swarm

import (
	"strings"
	"time"

	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/yiduoyunQ/smlib"
	crontab "gopkg.in/robfig/cron.v2"
)

const (
	_BackupWaiting = iota
	_BackupRunning
	_BackupDisabled
)

type serviceBackup struct {
	id       crontab.EntryID
	strategy *database.BackupStrategy
	schedule crontab.Schedule

	svc *Service
}

func NewBackupJob(svc *Service) *serviceBackup {
	return &serviceBackup{
		svc:      svc,
		strategy: svc.backup,
	}
}

func (bs *serviceBackup) Run() {
	strategy, err := database.GetBackupStrategy(bs.strategy.ID)
	if err != nil {
		return
	}

	if !strategy.Enabled || strategy.Status != _BackupWaiting {
		return
	}

	task := database.NewTask("backup_strategy", strategy.ID, "", nil, strategy.Timeout)
	strategy.Status = _BackupRunning
	task.Status = 1

	err = database.TXUpdateBackupJob(strategy, task)
	if err != nil {
		return
	}

	bs.strategy = strategy

	addr, port, err := bs.svc.GetSwithManagerAddr()
	if err != nil {
		return
	}

	topology, err := smlib.GetTopology(addr, port)
	if err != nil {
		return
	}

	masterID := ""
loop:
	for _, val := range topology.DataNodeGroup {
		for id, node := range val {
			if strings.EqualFold(node.Type, "master") {
				masterID = id

				break loop
			}
		}
	}

	if masterID == "" {
		// Not Found master DB
		return
	}

	bs.svc.RLock()

	master, err := bs.svc.getUnit(masterID)

	bs.svc.RUnlock()

	if err := smlib.Lock(addr, port); err != nil {
		return
	}
	defer smlib.UnLock(addr, port)

	args := []string{"callback url", task.ID, strategy.ID, master.ID, strategy.Type}

	errCh := make(chan error, 1)
	select {
	case errCh <- master.backup(args...):

	case <-time.After(strategy.Timeout):

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
		strategy.UpdateNext(next, false, _BackupDisabled)
		bs.strategy = strategy

		return time.Time{}
	}

	err = strategy.UpdateNext(next, true, _BackupWaiting)
	if err != nil {
		return time.Time{}
	}

	bs.strategy = strategy

	return next
}

func (gd *Gardener) RegisterBackupStrategy(strategy *serviceBackup) error {
	id := gd.cron.Schedule(strategy, strategy)
	strategy.id = id

	gd.Lock()
	gd.cronJobs[id] = strategy
	gd.Unlock()

	return nil
}

func (gd *Gardener) RemoveCronJob(strategyID string) error {
	gd.Lock()

	for key, val := range gd.cronJobs {
		if val.strategy.ID == strategyID {
			gd.cron.Remove(key)
			gd.Unlock()

			return nil
		}
	}

	gd.Unlock()

	return nil
}
