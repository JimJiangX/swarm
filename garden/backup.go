package garden

import (
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (svc *Service) Backup(ctx context.Context, local string, config structs.ServiceBackupConfig, async bool, task *database.Task) error {
	backup := func() error {
		var (
			err   error
			units []*unit
		)
		if config.Container != "" {
			var u *unit
			u, err = svc.getUnit(config.Container)
			units = []*unit{u}
		} else {
			units, err = svc.getUnits()
		}
		if err != nil {
			return err
		}

		cmds, err := svc.generateUnitsCmd(ctx)
		if err != nil {
			return err
		}

		for _, u := range units {
			cmd := cmds.GetCmd(u.u.ID, structs.BackupCmd)
			if len(cmd) == 0 {
				return errors.Errorf("%s:%s unsupport backup yet", u.u.Name, u.u.Type)
			}

			cmd = append(cmd, local+"v1.0/tasks/backup/callback", task.ID, config.Type, config.BackupDir)

			_, err = u.containerExec(ctx, cmd, config.Detach)
			if err != nil {
				return err
			}
		}

		return nil
	}

	sl := tasklock.NewServiceTask(svc.svc.ID, svc.so, task,
		statusServiceBackuping, statusServiceBackupDone, statusServiceBackupFailed)

	return sl.Run(isnotInProgress, backup, async)
}
