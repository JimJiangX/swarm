package garden

import (
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"golang.org/x/net/context"
)

func (svc *Service) UnitRestore(ctx context.Context, unit, path string, async bool) (string, error) {
	do := func() error {
		sys, err := svc.so.GetSysConfig()
		if err != nil {
			return err
		}

		cmds, err := svc.generateUnitsCmd(ctx)
		if err != nil {
			return err
		}

		u, err := svc.getUnit(unit)
		if err != nil {
			return err
		}

		// TODO:stop service
		err = u.stopContainer(ctx)
		if err != nil {
			return err
		}

		err = u.startContainer(ctx)
		if err != nil {
			return err
		}

		cmd := cmds.GetCmd(u.u.ID, structs.RestoreCmd)

		cmd = append(cmd, path, sys.BackupDir)

		_, err = u.containerExec(ctx, cmd, false)
		if err != nil {
			return err
		}

		// start service
		cmd = cmds.GetCmd(u.u.ID, structs.StartServiceCmd)

		_, err = u.containerExec(ctx, cmd, false)

		return err
	}

	t := database.NewTask(svc.svc.Name, database.UnitRestoreTask, svc.svc.ID, unit, nil, 300)
	tl := tasklock.NewServiceTask(svc.svc.ID, svc.so, &t,
		statusServiceRestoring,
		statusServiceRestored,
		statusServiceRestoreFailed)

	err := tl.Run(isnotInProgress, do, async)

	return t.ID, err
}
