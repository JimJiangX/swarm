package cluster

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// IsErrVolumeNotFound returns true if the error is caused
// when a volume is not found in the docker host.
func IsErrVolumeNotFound(err error) bool {
	if client.IsErrVolumeNotFound(err) {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "no such volume")
}

// IsErrContainerNotFound returns true if the error is caused
// when a container is not found in the docker host.
func IsErrContainerNotFound(err error) bool {
	if err == nil {
		return false
	}

	if client.IsErrContainerNotFound(err) {
		return true
	}

	if strings.Contains(err.Error(), "No such container") {
		return true
	}

	return false
}

// ServerVersion returns information of the docker client and server host.
func (e *Engine) ServerVersion() (types.Version, error) {
	return e.apiClient.ServerVersion(context.Background())
}

// UsedCpus returns the sum of CPUs reserved by containers.
func (e *Engine) UsedCpus() int64 {
	var r int64
	e.RLock()
	for _, c := range e.containers {

		if c.Config.HostConfig.CpusetCpus == "" {
			r += c.Config.HostConfig.CPUShares
		} else {

			n, err := c.Config.CountCPU()
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"Host":      e.Name,
					"Engine":    e.Addr,
					"Container": c.Names,
				}).Errorf("parser Config.HostConfig.CpusetCpus:%s error:%s", c.Config.HostConfig.CpusetCpus, err)
			}

			r += n
		}
	}
	e.RUnlock()
	return r
}

// StopContainer stop a container
func (e *Engine) StopContainer(ctx context.Context, name string, timeout *time.Duration) error {
	container, err := e.getContainer(name)
	if err != nil {
		return err
	}

	err = e.apiClient.ContainerStop(ctx, container.ID, timeout)
	e.CheckConnectionErr(err)

	return errors.WithStack(err)
}

func (e *Engine) getContainer(name string) (*Container, error) {
	var container *Container

	e.RLock()
	for _, c := range e.containers {
		if c.ID == name {
			container = c
			break
		}
	}
	e.RUnlock()

	if container == nil {
		if container = e.Containers().Get(name); container == nil {
			return nil, errors.Errorf("not found container '%s' in Engine %s", name, e.Addr)
		}
	}

	return container, nil
}

// UpdateContainer updates a container with new UpdateConfig
func (e *Engine) UpdateContainer(ctx context.Context, name string, config container.UpdateConfig) (*Container, error) {
	container, err := e.getContainer(name)
	if err != nil {
		return nil, err
	}

	resp, err := e.apiClient.ContainerUpdate(ctx, container.ID, config)
	e.CheckConnectionErr(err)
	if err != nil {
		return container, errors.Wrapf(err, "container update,warnings:%s", resp.Warnings)
	}

	// Register the container immediately while waiting for a state refresh.
	// Force a state refresh to pick up the newly created container.
	e.refreshContainer(container.ID, true)

	e.Lock()
	container = e.containers[container.ID]
	e.Unlock()

	if container == nil {
		err = errors.New("Container update but refresh didn't report it back")
	}

	return container, err
}

// Exec returns the container exec command result
func (c Container) Exec(ctx context.Context, cmd []string, detach bool, w io.Writer) (types.ContainerExecInspect, error) {
	if c.Engine == nil {
		return types.ContainerExecInspect{}, errors.Errorf("Engine of Container:%s is required,command=%s", c.Names, cmd)
	}

	inspect, err := c.Engine.containerExec(ctx, c.ID, cmd, detach, w)
	if err == nil {
		return inspect, nil
	}

	return inspect, errors.Wrapf(err, "command=%s", cmd)
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
func (e *Engine) containerExec(ctx context.Context, containerID string, cmd []string, detach bool, w io.Writer) (types.ContainerExecInspect, error) {
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

	// We need to check the tty _before_ we do the ContainerExecCreate, because
	// otherwise if we error out we will leak execIDs on the server (and
	// there's no easy way to clean those up). But also in order to make "not
	// exist" errors take precedence we do a dummy inspect first.
	c, err := e.apiClient.ContainerInspect(ctx, containerID)
	e.CheckConnectionErr(err)
	if err != nil {
		return inspect, errors.WithStack(err)
	}

	containerID = c.ID

	exec, err := e.apiClient.ContainerExecCreate(ctx, containerID, execConfig)
	e.CheckConnectionErr(err)
	if err != nil {
		return inspect, errors.Wrapf(err, "Container %s exec create", containerID)
	}

	{
		// add execID to the container, so the later exec/start will work
		container := e.Containers().Get(containerID)
		container.Info.ExecIDs = append(container.Info.ExecIDs, exec.ID)
	}

	// TODO: remove
	logrus.WithFields(logrus.Fields{
		"Container": containerID,
		"Engine":    e.Addr,
		"ExecID":    exec.ID,
		"Detach":    execConfig.Detach,
	}).Debugf("start exec:%s", cmd)

	if execConfig.Detach {
		err := e.apiClient.ContainerExecStart(ctx, exec.ID, types.ExecStartCheck{Detach: detach})
		e.CheckConnectionErr(err)
		if err != nil {
			return inspect, errors.Wrapf(err, "Container %s exec start %s", containerID, exec.ID)
		}
	} else {
		if err = checkTtyInput(execConfig.AttachStdin, execConfig.Tty); err != nil {
			logrus.Warn(err)
		}

		err = e.containerExecAttch(ctx, exec.ID, execConfig, w)
		if err != nil {
			return inspect, err
		}

		status := 0
		inspect, status, err = e.getExecExitCode(ctx, exec.ID)
		if err != nil {
			return inspect, err
		}
		if status != 0 {
			err = errors.Errorf("Container %s,Engine %s:%s,ExecID %s,ExitCode:%d,ExecInspect:%v", containerID, e.Name, e.Addr, exec.ID, status, inspect)
		}
	}

	return inspect, err
}

func (e *Engine) containerExecAttch(ctx context.Context, execID string, execConfig types.ExecConfig, w io.Writer) error {
	var (
		out, stderr io.Writer     = os.Stdout, os.Stderr
		in          io.ReadCloser = os.Stdin
	)

	resp, err := e.apiClient.ContainerExecAttach(ctx, execID, execConfig)
	e.CheckConnectionErr(err)
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Close()

	if w != nil {
		out, stderr = w, w
	}

	err = holdHijackedConnection(ctx, execConfig.Tty, in, out, stderr, resp)
	if err != nil {
		return err
	}

	return nil
}

// getExecExitCode perform an inspect on the exec command. It returns ContainerExecInspect.
func (e *Engine) getExecExitCode(ctx context.Context, execID string) (types.ContainerExecInspect, int, error) {
	resp, err := e.apiClient.ContainerExecInspect(ctx, execID)
	e.CheckConnectionErr(err)
	if err != nil {
		// If we can't connect, then the daemon probably died.
		if client.IsErrConnectionFailed(err) {
			return types.ContainerExecInspect{}, -1, errors.WithStack(err)
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
			return errors.WithStack(err)
		}
	case <-stdinDone:
		if outputStream != nil || errorStream != nil {
			select {
			case err := <-receiveStdout:
				if err != nil {
					return errors.WithStack(err)
				}
			case <-ctx.Done():
			}
		}
	case <-ctx.Done():
	}

	return nil
}
