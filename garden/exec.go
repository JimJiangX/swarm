package garden

import (
	"bytes"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	"golang.org/x/net/context"
)

// ExecLock service run containers exec process with locked.
func (svc *Service) ExecLock(exec func() error, async bool, task *database.Task) error {
	sl := tasklock.NewServiceTask(database.ServiceExecTask, svc.ID(), svc.so, task,
		statusServiceExecStart, statusServiceExecDone, statusServiceExecFailed)

	return sl.Run(isnotInProgress, exec, async)
}

// ContainerExec service run containers exec command.
func (svc *Service) ContainerExec(ctx context.Context, nameOrID string, cmd []string, detach bool) ([]structs.ContainerExecOutput, error) {
	var units []*unit

	if nameOrID != "" {
		u, err := svc.GetUnit(nameOrID)
		if err != nil {
			return nil, err
		}
		units = []*unit{u}
	} else {
		list, err := svc.getUnits()
		if err != nil {
			return nil, err
		}

		units = list
	}

	out := make([]structs.ContainerExecOutput, 0, len(units))
	buf := bytes.NewBuffer(nil)

	for _, u := range units {

		inspect, err := u.ContainerExec(ctx, cmd, detach, buf)
		out = append(out, structs.ContainerExecOutput{
			Code:   inspect.ExitCode,
			Unit:   u.u.ID,
			Output: buf.String(),
		})
		if err != nil {
			return out, err
		}

		buf.Reset()
	}

	return out, nil
}
