package garden

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/seed/sdk"
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
	u       database.Unit
	uo      database.UnitOrmer
	cluster cluster.Cluster
}

func newUnit(u database.Unit, uo database.UnitOrmer, cluster cluster.Cluster) *unit {
	return &unit{
		u:       u,
		uo:      uo,
		cluster: cluster,
	}
}

func (u unit) getContainer() *cluster.Container {
	if u.u.ContainerID != "" {
		return u.cluster.Container(u.u.ContainerID)
	}

	return u.cluster.Container(u.u.Name)
}

func (u unit) containerIDOrName() string {
	name := u.u.ContainerID
	if name == "" {
		name = u.u.Name
	}

	return name
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

func (u unit) prepareExpandVolume(target []structs.VolumeRequire) ([]structs.VolumeRequire, error) {
	lvs, err := u.uo.ListVolumesByUnitID(u.u.ID)
	if err != nil {
		return nil, err
	}

	add := make([]structs.VolumeRequire, len(target))

	for i := range target {
		found := false
		add[i] = target[i]

	loop:
		for v := range lvs {
			if lvs[v].DriverType == target[i].Type &&
				strings.Contains(lvs[v].Name, target[i].Name) {

				found = true
				add[i].Size = target[i].Size - lvs[v].Size

				break loop
			}
		}

		if !found {
			return nil, errors.Errorf("unit:%s not found volume '%s_%s'", u.u.Name, target[i].Type, target[i].Name)
		}
	}

	return add, nil
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

	return "", errors.WithStack(newContainerError(u.u.Name, "host IP is required"))
}

// create networking after start container
func (u unit) startContainer(ctx context.Context) error {
	c := u.getContainer()
	if c == nil {
		return errors.WithStack(newNotFound("Container", u.u.Name))
	}

	state := c.Info.State

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	err := c.Engine.StartContainer(c)
	if err != nil {
		return errors.Wrap(err, "start container:"+u.u.Name)
	}

	// start networking

	sys, err := u.uo.GetSysConfig()
	if err != nil {
		return err
	}

	ips, err := u.uo.ListIPByUnitID(u.u.ID)
	if err != nil {
		return err
	}
	if len(ips) > 0 {
		addr := net.JoinHostPort(c.Engine.IP, strconv.Itoa(sys.Ports.SwarmAgent))
		err := u.startNetwork(ctx, addr, c.ID, ips, nil)
		if err != nil {
			if state != nil && state.Running {
				return nil
			}

			return err
		}
	}

	return nil
}

func (u unit) startService(ctx context.Context, cmd []string) error {
	err := u.startContainer(ctx)
	if err != nil {
		return err
	}

	_, err = u.containerExec(ctx, cmd, false)

	return err
}

func (u unit) stopService(ctx context.Context, cmd []string, container bool) error {
	_, err := u.containerExec(ctx, cmd, false)
	if err != nil {
		return err
	}

	if !container {
		return nil
	}

	return u.stopContainer(ctx)
}

func (u unit) stopContainer(ctx context.Context) error {
	engine := u.getEngine()
	if engine == nil {
		return nil
	}

	timeout := 30 * time.Second
	err := engine.StopContainer(ctx, u.containerIDOrName(), &timeout)

	return err
}

func (u unit) removeContainer(ctx context.Context, rmVolumes, force bool) error {
	engine := u.getEngine()
	if engine == nil {
		if force || u.u.EngineID == "" {
			return nil
		}

		return errors.WithStack(newNotFound("Engine", u.u.EngineID))
	}

	c := u.getContainer()
	if c == nil {
		err := engine.RemoveContainer(&cluster.Container{
			Container: types.Container{ID: u.containerIDOrName()}}, force, rmVolumes)
		if err != nil {
			if cluster.IsErrContainerNotFound(err) || (force && !engine.IsHealthy()) {
				return nil
			}

			return err
		}
	}

	if !force {
		timeout := 30 * time.Second
		err := engine.StopContainer(ctx, c.ID, &timeout)
		if err != nil {
			if cluster.IsErrContainerNotFound(err) {
				return nil
			}

			return err
		}
	}

	err := engine.RemoveContainer(c, force, rmVolumes)
	if err != nil && cluster.IsErrContainerNotFound(err) {
		return nil
	}

	return errors.WithStack(err)
}

func (u unit) removeVolumes(ctx context.Context) error {
	lvs, err := u.getVolumesByUnit()
	if err != nil {
		return err
	}
	if len(lvs) == 0 {
		return nil
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	logrus.WithFields(logrus.Fields{
		"Unit":   u.u.Name,
		"Engine": u.u.EngineID,
	}).Info("remove container volumes...")

	engine := u.getEngine()
	if engine == nil {
		for i := range lvs {
			ok, err := u.cluster.RemoveVolumes(lvs[i].Name)
			if !ok {
				continue
			}
			if err != nil {
				return errors.WithStack(err)
			}
		}

		return nil
	}

	for i := range lvs {
		err := engine.RemoveVolume(lvs[i].Name)
		if err != nil {
			ok, err := u.cluster.RemoveVolumes(lvs[i].Name)
			if !ok {
				continue
			}
			return errors.WithStack(err)
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
		return types.ContainerExecInspect{}, errors.WithStack(newContainerError(u.u.Name, "not found"))
	}

	if !c.Info.State.Running {
		return types.ContainerExecInspect{}, errors.WithStack(newContainerError(u.u.Name, "is not running"))
	}

	return c.Exec(ctx, cmd, detach)
}

func (u unit) updateServiceConfig(ctx context.Context, path, context string) error {
	cmd := []string{"/bin/sh", "-c", fmt.Sprintf(`echo "%s" > %s`, context, path)}

	inspect, err := u.containerExec(ctx, cmd, false)
	if err != nil {
		logrus.WithField("Container", u.u.Name).Errorf("update config file %s,%#v,%+v", path, inspect, err)
	}

	return err
}

func (u *unit) update(ctx context.Context, config container.UpdateConfig) error {
	e := u.getEngine()
	if e == nil {
		return errors.Errorf("not found Engine %s", u.u.EngineID)
	}

	_, err := e.UpdateContainer(ctx, u.u.ContainerID, config)

	return err
}

func (u unit) startNetwork(ctx context.Context, addr, container string, ips []database.IP, tlsConfig *tls.Config) error {
	return createNetworkDevice(ctx, addr, container, ips, tlsConfig)
}

func createNetworkDevice(ctx context.Context, addr, container string, ips []database.IP, tlsConfig *tls.Config) error {
	for i := range ips {
		config := sdk.NetworkConfig{
			Container:  container,
			HostDevice: ips[i].Bond,
			// ContainerDevice: ips[i].Bond,
			IPCIDR:    fmt.Sprintf("%s/%d", utils.Uint32ToIP(ips[i].IPAddr), ips[i].Prefix),
			Gateway:   ips[i].Gateway,
			VlanID:    ips[i].VLAN,
			BandWidth: ips[i].Bandwidth,
		}

		err := postCreateNetwork(ctx, addr, config, tlsConfig)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func postCreateNetwork(ctx context.Context, addr string, config sdk.NetworkConfig, tlsConfig *tls.Config) error {
	cli, err := sdk.NewClient(addr, 30*time.Second, tlsConfig)
	if err != nil {
		return err
	}

	return cli.CreateNetwork(ctx, config)
}
