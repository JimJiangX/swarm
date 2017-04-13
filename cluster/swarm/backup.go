package swarm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
	"github.com/yiduoyunQ/smlib"
	"golang.org/x/net/context"
	crontab "gopkg.in/robfig/cron.v2"
)

type serviceBackup struct {
	id       crontab.EntryID
	strategy *database.BackupStrategy
	schedule crontab.Schedule
	svc      *Service
}

func newBackupJob(svc *Service) *serviceBackup {
	return &serviceBackup{
		svc:      svc,
		strategy: svc.backup,
	}
}

func (bs *serviceBackup) Run() {
	strategy, err := database.GetBackupStrategy(bs.strategy.ID)
	if err != nil {
		logrus.Warnf("BackupJob Run,%+v", err)
		return
	}

	if !strategy.Enabled || bs.svc == nil {
		return
	}

	ok, err := checkBackupFiles(strategy.ServiceID)
	if !ok || err != nil {
		logrus.Infof("Backup Task Canceled,%+v", err)
		return
	}

	bs.strategy = strategy

	task := database.NewTask(bs.svc.Name, backupAutoTask, strategy.ID, "", nil, strategy.Timeout)
	task.Status = statusTaskCreate
	err = task.Insert()
	if err != nil {
		logrus.Warnf("BackupJob Run,%+v", err)
		return
	}

	bs.svc.tryBackupTask(*strategy, &task)
}

func (bs *serviceBackup) Next(time.Time) time.Time {
	next := time.Time{}

	if bs.strategy == nil {
		return next
	}

	strategy, err := database.GetBackupStrategy(bs.strategy.ID)
	if err != nil {
		logrus.Warnf("BackupJob Next,%+v", err)
		return next
	}

	if bs.schedule == nil {
		bs.schedule, err = crontab.Parse(bs.strategy.Spec)
		if err != nil {
			logrus.Warnf("BackupJob Next,Parse %s,%+v", bs.strategy.Spec, err)
			return next
		}
	}

	next = bs.schedule.Next(time.Now())

	if next.IsZero() || next.After(strategy.Valid) {
		err := strategy.UpdateNext(next, false)
		if err != nil {
			logrus.Warnf("BackupJob Next,%+v", err)
		}
		bs.strategy = strategy

		return time.Time{}
	}

	err = strategy.UpdateNext(next, true)
	if err != nil {
		logrus.Warnf("BackupJob Next,%+v", err)
		return next
	}

	bs.strategy = strategy

	return next
}

func (svc *Service) tryBackupTask(strategy database.BackupStrategy, task *database.Task) error {
	addr, port, master, err := lockSwitchManager(svc, 3)
	if err != nil {
		err1 := database.UpdateTaskStatus(task, statusTaskCancel, time.Now(), "Cancel,"+err.Error())
		err = errors.Wrapf(err, "update Task status errors:%v", err1)
		logrus.Error(err)

		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(strategy.Timeout)*time.Second)
	defer cancel()

	err = backupTask(ctx, master, task, strategy, func() error {
		var err error
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
		err = smlib.UnLock(addr, port)
		if err != nil {
			logrus.Errorf("switch_manager %s:%d Unlock Error:%s", addr, port, err)
		}

		return err
	})
	if err != nil {
		logrus.Errorf("%s backupTask error:%s", svc.Name, err)
	}

	return err
}

func lockSwitchManager(svc *Service, retries int) (string, int, *unit, error) {
	var (
		addr   string
		port   int
		master *unit
		err    error
	)

	for count := 0; count < retries; count++ {
		addr, port, master, err = svc.getSwitchManagerAndMaster()
		if err != nil || master == nil {
			logrus.WithField("Service", svc.Name).WithError(err).Errorf("get SwitchManager and Master,retries=%d", retries)
			continue
		}

		err = smlib.Lock(addr, port)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"Service": svc.Name,
				"addr":    addr,
				"port":    port,
			}).WithError(err).Error("lock SwitchManager")

			continue
		}

		break
	}

	return addr, port, master, errors.Wrap(err, "lock switch manager")
}

func backupTask(ctx context.Context, backup *unit, task *database.Task,
	strategy database.BackupStrategy, after func() error) error {
	if after != nil {
		defer after()
	}

	entry := logrus.WithFields(logrus.Fields{
		"Unit":     backup.Name,
		"Strategy": strategy.ID,
		"Task":     task.ID,
	})

	args := []string{hostAddress + ":" + httpPort + "/v1.0/tasks/backup/callback", task.ID, strategy.ID, backup.ID, strategy.Type, strategy.BackupDir}

	msg, status := "", int64(0)

	err := backup.backup(ctx, args...)
	if err != nil {
		backup.Status = statusUnitBackupFailed
		backup.LatestError = err.Error()
	} else {
		backup.Status = statusUnitBackuped
		backup.LatestError = ""

		_err := database.TxUpdateUnitStatus(&backup.Unit, backup.Status, backup.LatestError)
		if _err != nil {
			entry.WithError(_err).Warnf("update backup status=%d,error=%s", backup.Status, backup.LatestError)
		}

		entry.Info("backup done")
		return err
	}

	status = statusTaskFailed
	msg = fmt.Sprintf("Backup Task Faild,%s", err)
	entry.Error(msg)

	select {
	case <-ctx.Done():
		if ctxErr := ctx.Err(); ctxErr != nil {
			if ctxErr == context.DeadlineExceeded {
				msg = "Timeout," + msg
				status = statusTaskTimeout
			} else if ctxErr == context.Canceled {
				msg = "Canceled," + msg
				status = statusTaskCancel
			}
		}
	default:
	}

	task.Status = status
	_err := database.TxUpdateUnitStatusWithTask(&backup.Unit, task, msg)
	if _err != nil {
		entry.Errorf("Update TaskStatus Error:%s,message=%s", _err, msg)
	}

	return err
}

func removeExpiredBackupFiles() error {
	sys, err := database.GetSystemConfig()
	if err != nil {
		return err
	}

	if _, err := os.Stat(sys.NFSOption.MountDir); os.IsNotExist(err) {
		logrus.Errorf("nfs:'%s',%s", sys.NFSOption.MountDir, err)
		return err
	}

	files, err := database.ListBackupFiles()
	if err != nil {
		return err
	}

	now := time.Now()
	expired := make([]database.BackupFile, 0, len(files))

	for i := range files {
		if now.After(files[i].Retention) {
			expired = append(expired, files[i])
		}
	}

	for i := range expired {
		abs, err := abs(sys.NFSOption.MountDir, sys.BackupDir, expired[i].Path)
		if err != nil {
			logrus.Errorf("RemoveAll expired backup file %s,%s", expired[i].Path, err)
			continue
		}

		logrus.Infof("Backup files expired:%v", abs)

		err = os.RemoveAll(abs)
		if err != nil {
			logrus.Errorf("RemoveAll expired backup file %s,%s", abs, err)
			continue
		}

		err = database.DelBackupFile(expired[i].ID)
		if err != nil {
			logrus.Errorf("del expired backup file,%+v", err)
		}
	}

	return err
}

func abs(prefix, base, path string) (string, error) {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", errors.Wrap(err, path)
	}

	return filepath.Join(prefix, rel), nil
}

// checkBackupFiles checks files.Retention,remove expired files
func checkBackupFiles(nameOrID string) (bool, error) {
	removeExpiredBackupFiles()

	service, files, err := database.ListBackupFilesByService(nameOrID)
	if err != nil {
		return false, err
	}

	now := time.Now()
	valid := make([]database.BackupFile, 0, len(files))

	for i := range files {
		if now.Before(files[i].Retention) {
			valid = append(valid, files[i])
		}
	}

	sub := 0
	for i := range valid {
		sub += valid[i].SizeByte
	}

	if sub >= service.BackupMaxSizeByte {
		logrus.WithFields(logrus.Fields{
			"Service": service.Name,
			"Max":     service.BackupMaxSizeByte,
			"Used":    sub,
		}).Warning("no more space for backup task")

		return false, nil
	}

	return true, nil
}

func (gd *Gardener) registerBackupStrategy(strategy *serviceBackup) error {
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

func (gd *Gardener) removeCronJob(strategyID string) error {
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

// ReplaceServiceBackupStrategy returns a new *database.BackupStrategy,replace original service backupstrategy
func (gd *Gardener) ReplaceServiceBackupStrategy(nameOrID string, req structs.BackupStrategy) (*database.BackupStrategy, error) {
	svc, err := gd.GetService(nameOrID)
	if err != nil {
		return nil, err
	}

	sys, err := gd.systemConfig()
	if err != nil {
		logrus.WithField("Service", svc.Name).Warn(err)
	}

	req.BackupDir = sys.BackupDir

	strategy, err := svc.replaceBackupStrategy(req)
	if err != nil {
		return strategy, err
	}

	if svc.backup != nil {
		bs := newBackupJob(svc)
		err = gd.registerBackupStrategy(bs)
		if err != nil {
			logrus.WithField("Service", svc.Name).WithError(err).Error("Add BackupStrategy to Gardener.Crontab")
		}
	}

	return strategy, err
}

// UpdateServiceBackupStrategy update assigned BackupStrategy
func (gd *Gardener) UpdateServiceBackupStrategy(nameOrID string, req structs.BackupStrategy) error {
	var (
		valid = time.Time{}
		err   error
	)
	if req.Valid != "" {
		valid, err = utils.ParseStringToTime(req.Valid)
		if err != nil {
			logrus.Error("Parse BackupStrategy.Valid from string to Time", err)
			return errors.Wrap(err, "parse BackupStrategy.Valid")
		}
	}

	sys, err := gd.systemConfig()
	if err != nil {
		logrus.Errorf("%+v", err)
	}

	req.BackupDir = sys.BackupDir

	bs, err := database.GetBackupStrategy(nameOrID)
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
		Timeout:   req.Timeout,
		Next:      bs.Next,
		Enabled:   req.Enable,
		CreatedAt: bs.CreatedAt,
	}

	err = database.UpdateBackupStrategy(update)

	return err
}

// EnableServiceBackupStrategy enable backupStrategy
func (gd *Gardener) EnableServiceBackupStrategy(strategy string) error {
	backup, err := database.GetBackupStrategy(strategy)
	if err != nil || backup == nil {
		return errors.Errorf("not found BackupStrategy,%+v", err)
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

		bs := newBackupJob(svc)
		gd.registerBackupStrategy(bs)
	}

	return err
}

// DisableBackupStrategy disable a backupStrategy
func (gd *Gardener) DisableBackupStrategy(ID string) error {

	return database.UpdateBackupStrategyStatus(ID, false)
}

// BackupTaskCallback update backup task and save new backup file
func BackupTaskCallback(req structs.BackupTaskCallback) error {
	task := database.Task{ID: req.TaskID}

	if rerr := req.Error(); rerr != nil {
		err := database.UpdateTaskStatus(&task, structs.TaskFailed, time.Now(), req.Error().Error())
		if err != nil {
			return err
		}

		return errors.Wrap(err, "from request error")
	}

	task, rent, err := database.BackupTaskValidate(req.TaskID, req.StrategyID, req.UnitID)
	if err != nil {

		return err
	}

	backupFile := database.BackupFile{
		ID:         utils.Generate64UUID(),
		TaskID:     task.ID,
		StrategyID: req.StrategyID,
		UnitID:     req.UnitID,
		Type:       req.Type,
		Path:       req.Path,
		SizeByte:   req.Size,
		CreatedAt:  task.CreatedAt, // task.CreatedAt
		FinishedAt: time.Now(),
	}

	if rent > 0 {
		backupFile.Retention = backupFile.CreatedAt.AddDate(0, 0, rent)
	}

	err = database.TxBackupTaskDone(&task, statusTaskDone, backupFile)

	return err
}
