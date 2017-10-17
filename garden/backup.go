package garden

import (
	"strconv"
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Backup is exported
// 对服务进行备份，如果指定则对指定的容器进行备份，执行ContainerExec进行备份任务。
func (svc *Service) Backup(ctx context.Context, local string, config structs.ServiceBackupConfig, async bool, task *database.Task) error {
	backup := func() error {
		//		err := svc.checkBackupFiles(ctx, config.MaxSizeByte)
		//		if err != nil {
		//			return err
		//		}
		var (
			u   *unit
			err error
		)

		if config.Container != "" {
			u, err = svc.getUnit(config.Container)
		}
		if err != nil {
			return err
		}

		cmds, err := svc.generateUnitsCmd(ctx)
		if err != nil {
			return err
		}

		cmd := cmds.GetCmd(u.u.ID, structs.BackupCmd)
		if len(cmd) == 0 {
			return errors.Errorf("%s:%s unsupport backup yet", u.u.Name, u.u.Type)
		}

		cmd = append(cmd, local+"/v1.0/tasks/backup/callback", task.ID, u.u.ID, config.Type, config.BackupDir,
			strconv.Itoa(config.FilesRetention), config.Remark, config.Tag)

		_, err = u.containerExec(ctx, cmd, config.Detach)

		return err
	}

	sl := tasklock.NewServiceTask(svc.ID(), svc.so, task,
		statusServiceBackuping, statusServiceBackupDone, statusServiceBackupFailed)

	return sl.Run(isnotInProgress, backup, async)
}

func (svc *Service) checkBackupFiles(ctx context.Context, maxSize int) error {
	_, expired, err := checkBackupFilesByService(svc.ID(), svc.so, maxSize)
	if len(expired) > 0 {
		_err := svc.removeExpiredBackupFiles(ctx, expired)
		if _err != nil {
			return _err
		}

		err = svc.so.DelBackupFiles(expired)
	}

	return err
}

func (svc *Service) removeExpiredBackupFiles(ctx context.Context, files []database.BackupFile) error {
	for i := range files {
		u, err := svc.getUnit(files[i].UnitID)
		if err != nil {
			return err
		}

		cmd := []string{"rm", "-rf", files[i].Path}

		_, err = u.containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkBackupFilesByService(service string, iface database.BackupFileIface, maxSize int) ([]database.BackupFile, []database.BackupFile, error) {
	files, err := iface.ListBackupFilesByService(service)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	valid := make([]database.BackupFile, 0, len(files))
	expired := make([]database.BackupFile, 0, len(files))

	for i := range files {
		if now.Sub(files[i].Retention) > -time.Minute {
			expired = append(expired, files[i])
		} else {
			valid = append(valid, files[i])
		}
	}

	// check used space if space is limited
	if maxSize > 0 {
		sum := 0
		for i := range valid {
			sum += valid[i].SizeByte
		}

		if sum > maxSize {
			return valid, expired, errors.Errorf("no more space for backup task,%d<%d", maxSize, sum)
		}
	}

	return valid, expired, nil
}
