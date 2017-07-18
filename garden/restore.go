package garden

import (
	"strings"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (u unit) restore(ctx context.Context, path, backupDir string, cmds structs.Commands) error {
	cmd := cmds.GetCmd(u.u.ID, structs.StopServiceCmd)
	err := u.stopService(ctx, cmd, false)
	if err != nil {
		return err
	}

	err = u.startContainer(ctx)
	if err != nil {
		return err
	}

	cmd = cmds.GetCmd(u.u.ID, structs.RestoreCmd)

	cmd = append(cmd, path, backupDir)

	_, err = u.containerExec(ctx, cmd, false)
	if err != nil {
		return err
	}

	// start service
	cmd = cmds.GetCmd(u.u.ID, structs.StartServiceCmd)

	_, err = u.containerExec(ctx, cmd, false)

	return err

}

// UnitRestore resotore an unit volume data by the assigned backup file.
func (svc *Service) UnitRestore(ctx context.Context, assigned []string, path string, async bool) (string, error) {
	do := func() error {
		var units []*unit

		switch len(assigned) {
		case 0:
			return errors.New("restore unit without assigned")
		case 1:
			u, err := svc.getUnit(assigned[0])
			if err != nil {
				return err
			}
			units = []*unit{u}

		default:
			out, err := svc.getUnits()
			if err != nil {
				return err
			}

			units = make([]*unit, 0, len(assigned))

			for i := range assigned {
				found := false
				for k := range out {
					if out[k].u.ID == assigned[i] || out[k].u.Name == assigned[i] {
						found = true
						units = append(units, out[k])
					}
				}
				if !found {
					return errors.Errorf("%s is not belongs to service %s", assigned[i], svc.svc.Name)
				}
			}
		}

		sys, err := svc.so.GetSysConfig()
		if err != nil {
			return err
		}

		cmds, err := svc.generateUnitsCmd(ctx)
		if err != nil {
			return err
		}

		for _, u := range units {
			err := u.restore(ctx, path, sys.BackupDir, cmds)
			if err != nil {
				return err
			}
		}

		return err
	}

	t := database.NewTask(svc.svc.Name, database.UnitRestoreTask, svc.svc.ID, strings.Join(assigned, "&&"), nil, 300)
	tl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &t,
		statusServiceRestoring,
		statusServiceRestored,
		statusServiceRestoreFailed)

	err := tl.Run(isnotInProgress, do, async)

	return t.ID, err
}
