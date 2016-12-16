package cluster

import (
	"io"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// UsedCpus returns the sum of CPUs reserved by containers.
func (e *Engine) UsedCpus() int64 {
	var r int64
	e.RLock()
	for _, c := range e.containers {

		if c.Config.HostConfig.CpusetCpus == "" {
			r += c.Config.HostConfig.CPUShares
		} else {

			n, err := utils.CountCPU(c.Config.HostConfig.CpusetCpus)
			if err != nil {
				// TODO:
			}

			r += n
		}
	}
	e.RUnlock()
	return r
}

func (e *Engine) ContainerAPIClient() client.ContainerAPIClient {
	return e.apiClient
}

func (c Container) Exec(ctx context.Context, cmd []string) (types.ContainerExecInspect, error) {
	if c.Engine == nil || c.Info.State == nil || !c.Info.State.Running {
		return types.ContainerExecInspect{}, nil
	}

	return c.Engine.containerExec(ctx, c.ID, cmd, false)

}

// checkTtyInput checks if we are trying to attach to a container tty
// from a non-tty client input stream, and if so, returns an error.
func checkTtyInput(attachStdin, ttyMode bool) error {
	// In order to attach to a container tty, input stream for the client must
	// be a tty itself: redirecting or piping the client standard input is
	// incompatible with `docker run -t`, `docker exec -t` or `docker attach`.
	if ttyMode && attachStdin {
		return errors.New("cannot enable tty mode on non tty input")
	}
	return nil
}

// containerExec exec cmd in containeID,It returns ContainerExecInspect.
func (engine *Engine) containerExec(ctx context.Context, containerID string, cmd []string, detach bool) (types.ContainerExecInspect, error) {
	inspect := types.ContainerExecInspect{}

	execConfig := types.ExecConfig{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          cmd,
		Detach:       detach,
	}

	if detach {
		execConfig.AttachStderr = false
		execConfig.AttachStdout = false
	}

	exec, err := engine.apiClient.ContainerExecCreate(ctx, containerID, execConfig)
	engine.CheckConnectionErr(err)
	if err != nil {
		return inspect, errors.Wrapf(err, "Container %s exec create", containerID)
	}

	logrus.WithFields(logrus.Fields{
		"Container": containerID,
		"Engine":    engine.Addr,
		"ExecID":    exec.ID,
	}).Infof("start exec:%s", cmd)

	if execConfig.Detach {
		err := engine.apiClient.ContainerExecStart(ctx, exec.ID, types.ExecStartCheck{Detach: detach})
		engine.CheckConnectionErr(err)
		if err != nil {
			return inspect, errors.Wrapf(err, "Container %s exec start %s", containerID, exec.ID)
		}
	} else {
		if err = checkTtyInput(execConfig.AttachStdin, execConfig.Tty); err != nil {
			logrus.Warn(err)
		}

		err = engine.containerExecAttch(ctx, exec.ID, execConfig)
		engine.CheckConnectionErr(err)
		if err != nil {
			return inspect, err
		}

		status := 0
		inspect, status, err = engine.getExecExitCode(ctx, exec.ID)
		if err != nil {
			return inspect, err
		}
		if status != 0 {
			err = errors.Errorf("Container %s,Engine %s:%s,ExecID %s,ExitCode:%d,ExecInspect:%v", containerID, engine.Name, engine.Addr, exec.ID, status, inspect)
		}
	}

	return inspect, err
}

func (e *Engine) containerExecAttch(ctx context.Context, execID string, execConfig types.ExecConfig) error {
	var (
		out, stderr io.Writer     = os.Stdout, os.Stderr
		in          io.ReadCloser = os.Stdin
	)
	resp, err := e.apiClient.ContainerExecAttach(ctx, execID, execConfig)
	if err != nil {
		return errors.Wrap(err, "Container exec attch")
	}
	defer resp.Close()

	err = holdHijackedConnection(ctx, execConfig.Tty, in, out, stderr, resp)
	if err != nil {
		return err
	}

	return nil
}

// getExecExitCode perform an inspect on the exec command. It returns ContainerExecInspect.
func (e *Engine) getExecExitCode(ctx context.Context, execID string) (types.ContainerExecInspect, int, error) {
	resp, err := e.apiClient.ContainerExecInspect(ctx, execID)
	if err != nil {
		// If we can't connect, then the daemon probably died.
		if client.IsErrConnectionFailed(err) {
			return types.ContainerExecInspect{}, -1, errors.Wrap(err, "Container exec inspect")
		}
		return types.ContainerExecInspect{}, -1, nil
	}

	return resp, resp.ExitCode, nil
}

func holdHijackedConnection(ctx context.Context, tty bool, inputStream io.Reader, outputStream, errorStream io.Writer, resp types.HijackedResponse) error {
	receiveStdout := make(chan error, 1)
	if outputStream != nil || errorStream != nil {
		go func() {
			_, err := stdcopy.StdCopy(outputStream, errorStream, resp.Reader)
			logrus.Debugf("[hijack] End of stdout")
			receiveStdout <- err
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		if inputStream != nil {
			io.Copy(resp.Conn, inputStream)
			// we should restore the terminal as soon as possible once connection end
			// so any following print messages will be in normal type.

			logrus.Debugf("[hijack] End of stdin")
		}

		if err := resp.CloseWrite(); err != nil {
			logrus.Debugf("Couldn't send EOF: %s", err)
		}
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		if err != nil {
			logrus.Debugf("Error receiveStdout: %s", err)
			return errors.Wrap(err, "hijack receiveStdout")
		}
	case <-stdinDone:
		if outputStream != nil || errorStream != nil {
			select {
			case err := <-receiveStdout:
				if err != nil {
					logrus.Debugf("Error receiveStdout: %s", err)
					return errors.Wrap(err, "hijack receiveStdout")
				}
			case <-ctx.Done():
			}
		}
	case <-ctx.Done():
	}

	return nil
}
