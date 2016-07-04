package swarm

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/cluster"
	"golang.org/x/net/context"
)

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
func containerExec(ctx context.Context, engine *cluster.Engine, containerID string, cmd []string, detach bool) (types.ContainerExecInspect, error) {
	inspect := types.ContainerExecInspect{}
	client := engine.EngineAPIClient()
	if client == nil {
		return inspect, errEngineAPIisNil
	}

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

	exec, err := client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return inspect, err
	}

	logrus.WithFields(logrus.Fields{
		"Container":  containerID,
		"Engine":     engine.Addr,
		"ExecID":     exec.ID,
		"ExecConfig": execConfig,
	}).Info("Start Exec")

	if execConfig.Detach {
		err := client.ContainerExecStart(ctx, exec.ID, types.ExecStartCheck{Detach: detach})
		if err != nil {
			return inspect, err
		}
	} else {
		if err = checkTtyInput(execConfig.AttachStdin, execConfig.Tty); err != nil {
			logrus.Warn(err)
		}

		err = containerExecAttch(ctx, client, exec.ID, execConfig)
		if err != nil {
			return inspect, err
		}

		status := 0
		inspect, status, err = getExecExitCode(ctx, client, exec.ID)
		if err != nil {
			return inspect, err
		}
		if status != 0 {
			err = fmt.Errorf("Container %s Engine %s:%s ExecID %s,ExitCode:%d,ExecInspect:%v", containerID, engine.Name, engine.Addr, exec.ID, status, inspect)
		}
	}

	return inspect, err
}

func containerExecAttch(ctx context.Context, client client.APIClient, execID string, execConfig types.ExecConfig) error {
	var (
		out, stderr io.Writer     = os.Stdout, os.Stderr
		in          io.ReadCloser = os.Stdin
	)
	resp, err := client.ContainerExecAttach(ctx, execID, execConfig)
	if err != nil {
		return err
	}
	defer resp.Close()

	err = holdHijackedConnection(ctx, execConfig.Tty, in, out, stderr, resp)
	if err != nil {
		logrus.Debugf("Error hijack: %s", err)
		return err
	}

	return nil
}

// getExecExitCode perform an inspect on the exec command. It returns ContainerExecInspect.
func getExecExitCode(ctx context.Context, cli client.APIClient, execID string) (types.ContainerExecInspect, int, error) {
	resp, err := cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		// If we can't connect, then the daemon probably died.
		if err != client.ErrConnectionFailed {
			return types.ContainerExecInspect{}, -1, err
		}
		return types.ContainerExecInspect{}, -1, nil
	}

	return resp, resp.ExitCode, nil
}

func holdHijackedConnection(ctx context.Context, tty bool, inputStream io.ReadCloser, outputStream, errorStream io.Writer, resp types.HijackedResponse) error {
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
			return err
		}
	case <-stdinDone:
		if outputStream != nil || errorStream != nil {
			select {
			case err := <-receiveStdout:
				if err != nil {
					logrus.Debugf("Error receiveStdout: %s", err)
					return err
				}
			case <-ctx.Done():
			}
		}
	case <-ctx.Done():
	}

	return nil
}
