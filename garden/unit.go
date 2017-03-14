package garden

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/nic"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	statusUnitNoContent = iota

	statusUnitAllocting
	statusUnitAllocted
	statusUnitAlloctionFailed

	statusUnitCreating
	statusUnitCreated
	statusUnitCreateFailed

	statusUnitStarting // start contaier and start service
	statusUnitStarted
	statusUnitStartFailed

	statusUnitStoping
	statusUnitStoped
	statusUnitStopFailed

	statusUnitMigrating
	statusUnitMigrated
	statusUnitMigrateFailed

	statusUnitRebuilding
	statusUnitRebuilt
	statusUnitRebuildFailed

	statusUnitDeleting

	statusUnitBackuping
	statusUnitBackuped
	statusUnitBackupFailed

	statusUnitRestoring
	statusUnitRestored
	statusUnitRestoreFailed

	statusContainerCreated
	statusContainerRunning
	statusContainerPaused
	statusContainerRestarted
	statusContainerDead
	statusContainerExited
)

type errContainer struct {
	name   string
	action string
}

func (ec errContainer) Error() string {
	return fmt.Sprintf("Container %s %s", ec.name, ec.action)
}

func newContainerError(name, action string) errContainer {
	return errContainer{
		name:   name,
		action: action,
	}
}

type unit struct {
	u            database.Unit
	uo           database.UnitOrmer
	cluster      cluster.Cluster
	startNetwork func(ctx context.Context, unitID string, c *cluster.Container, orm database.NetworkingOrmer, tlsConfig *tls.Config) error
}

func newUnit(u database.Unit, uo database.UnitOrmer, cluster cluster.Cluster) *unit {
	return &unit{
		u:            u,
		uo:           uo,
		cluster:      cluster,
		startNetwork: nic.CreateNetworkDevice,
	}
}

//func (u unit) getNetworking() ([]Networking, error) {
//	return nil, nil
//}

func (u unit) getContainer() *cluster.Container {
	if u.u.ContainerID != "" {
		return u.cluster.Container(u.u.ContainerID)
	}

	return u.cluster.Container(u.u.Name)
}

func (u unit) getVolumesByUnit() ([]database.Volume, error) {
	lvs, err := u.uo.ListVolumesByUnitID(u.u.ID)

	return lvs, err
}

func (u unit) getEngine() *cluster.Engine {
	if u.u.EngineID != "" {
		return u.cluster.Engine(u.u.EngineID)
	}

	c := u.getContainer()
	if c != nil && c.Engine != nil {
		return c.Engine
	}

	return nil
}

func (u unit) getHostIP() (string, error) {
	engine := u.getEngine()
	if engine != nil {
		return engine.IP, nil
	}

	if u.u.EngineID != "" {
		n, err := u.uo.GetNode(u.u.EngineID)
		if err != nil {
			return "", err
		}

		parts := strings.SplitN(n.Addr, ":", 2)

		return parts[0], nil
	}

	return "", errors.Wrap(newContainerError(u.u.Name, "host IP is required"), "get host IP by unit")
}

func (u unit) startContainer(ctx context.Context) error {
	c := u.getContainer()
	if c == nil {
		return errors.Wrap(newNotFound("Container", u.u.Name), "container start")
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	err := c.Engine.StartContainer(c, nil)
	if err != nil {
		return errors.Wrap(err, "start container:"+u.u.Name)
	}

	// start networking
	if u.startNetwork != nil {
		err = u.startNetwork(ctx, u.u.ID, c, u.uo, nil)
	}

	return err
}

func (u unit) stopContainer(ctx context.Context) error {
	engine := u.getEngine()
	if engine == nil {
		return nil
	}

	client := engine.ContainerAPIClient()
	if client == nil {
		return nil
	}

	timeout := 30 * time.Second
	err := client.ContainerStop(ctx, u.u.Name, &timeout)
	engine.CheckConnectionErr(err)

	return err
}

func (u unit) removeContainer(ctx context.Context) error {
	c := u.getContainer()
	if c == nil {
		return nil
	}

	err := c.Engine.RemoveContainer(c, true, false)
	c.Engine.CheckConnectionErr(err)

	return err
}

func (u unit) removeVolumes(ctx context.Context) error {
	lvs, err := u.getVolumesByUnit()
	if err != nil {
		return err
	}
	if len(lvs) == 0 {
		return nil
	}

	engine := u.getEngine()
	if err != nil {
		return nil
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	for i := range lvs {
		err := engine.RemoveVolume(lvs[i].Name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u unit) containerExec(ctx context.Context, cmd []string, detach bool) (types.ContainerExecInspect, error) {
	if len(cmd) == 0 {
		return types.ContainerExecInspect{}, nil
	}
	c := u.getContainer()
	if c == nil {
		return types.ContainerExecInspect{}, errors.Wrap(newContainerError(u.u.Name, "not found"), "container exec")
	}

	if !c.Info.State.Running {
		return types.ContainerExecInspect{}, errors.Wrap(newContainerError(u.u.Name, "is not running"), "container exec")
	}

	return c.Exec(ctx, cmd, detach)
}

func (u unit) updateServiceConfig(ctx context.Context, path, context string) error {
	cmd := []string{"/bin/sh", "-c", fmt.Sprintf(`"echo '%s'> %s && chmod 644 %s"`, context, path, path)}

	inspect, err := u.containerExec(ctx, cmd, false)
	if err != nil {
		logrus.WithField("Container", u.u.Name).Errorf("update config file %s,%#v,%+v", path, inspect, err)
	}

	return err
}
