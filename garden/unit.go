package garden

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/resource/storage"
	"github.com/docker/swarm/garden/structs"
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
	statusContainerRenamed
	statusContainerRunning
	statusContainerPaused
	statusContainerRestarted
	statusContainerDead
	statusContainerExited
)

const (
	notRunning = "is not running"
	notFound   = "is not found"
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

func getContainer(cluster cluster.Cluster, cID, cName, engine string) *cluster.Container {
	if cID != "" {
		c := cluster.Container(cID)
		if c != nil {
			return c
		}
	}

	c := cluster.Container(cName)
	if c != nil {
		return c
	}

	if engine == "" {
		return nil
	}

	eng := cluster.Engine(engine)
	if eng == nil {
		return nil
	}

	err := eng.RefreshContainers(true)
	if err != nil {
		return nil
	}

	return eng.Containers().Get(cName)
}

func (u unit) getContainer() *cluster.Container {
	return getContainer(u.cluster, u.u.ContainerID, u.u.Name, u.u.EngineID)
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

func (u unit) prepareExpandVolume(eng *cluster.Engine, target []structs.VolumeRequire) ([]structs.VolumeRequire, []database.Volume, error) {
	lvs, err := u.uo.ListVolumesByUnitID(u.u.ID)
	if err != nil {
		return nil, nil, err
	}

	volumes := eng.Volumes()
	pending := make([]database.Volume, 0, len(lvs))

	add := make([]structs.VolumeRequire, len(target))

	for i := range target {
		found := false
		add[i] = target[i]

		for v := range lvs {
			if lvs[v].EngineID != eng.ID {
				continue
			}

			if volumes.Get(lvs[v].Name) == nil {

				// check volume size mayby changes
				if lvs[v].Size != target[i].Size &&
					strings.Contains(lvs[v].Name, target[i].Name) {

					lvs[v].Size = target[i].Size

					err := u.uo.SetVolume(lvs[v])
					if err != nil {
						return nil, nil, err
					}
				}

				pending = append(pending, lvs[v])
				continue
			}

			if strings.Contains(lvs[v].Name, target[i].Name) {
				found = true

				add[i].ID = lvs[v].ID
				add[i].Size = target[i].Size - lvs[v].Size

				if target[i].Type == "" {
					add[i].Type = lvs[v].DriverType
				}
			}
		}

		if !found && (target[i].Type == "" || target[i].Name == "") {
			return nil, nil, errors.Errorf("unit:%s not found volume '%s_%s'", u.u.Name, target[i].Type, target[i].Name)
		}
	}

	out := make([]structs.VolumeRequire, 0, len(add))
	for i := range add {
		if add[i].Size > 0 {
			out = append(out, add[i])
		}
	}

	return out, pending, nil
}

func (u unit) getHostIP() (string, error) {
	engine := u.getEngine()
	if engine != nil {
		return engine.IP, nil
	}

	if u.u.EngineID != "" {
		parts := strings.SplitN(u.u.EngineID, "|", 2)
		if len(parts) == 2 {
			host, _, err := net.SplitHostPort(parts[1])
			if err == nil {
				return host, nil
			}
		}
	}

	return "", errors.WithStack(newContainerError(u.u.Name, "host IP is required"))
}

// create networking after start container
func (u unit) startContainer(ctx context.Context) error {
	eng := u.getEngine()
	if eng == nil {
		return errors.WithStack(newNotFound("Engine by unit", u.u.Name))
	}

	err := u.cluster.RefreshEngine(eng.Name)
	if err != nil {
		return errors.Wrapf(err, "refresh engine:%s", eng.ID)
	}

	c := u.getContainer()
	if c == nil {
		return errors.WithStack(newNotFound("Container", u.u.Name))
	}

	u.u.ContainerID = c.ID

	if c.Info.State != nil && c.Info.State.Running {
		return nil
	}

	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	err = c.Engine.StartContainer(c)
	if err != nil {
		return errors.Wrap(err, "start container:"+u.u.Name)
	}

	// start networking
	err = u.startNetworking(ctx, c.Engine.IP, nil)

	return err
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
		// TODO：这里是冗余设计，是考虑到event事件处理可能会有延迟问题
		//		err := engine.RemoveContainer(&cluster.Container{
		//			Container: types.Container{ID: u.containerIDOrName()}}, force, rmVolumes)
		//		if err != nil {
		//			if cluster.IsErrContainerNotFound(err) || (force && !engine.IsHealthy()) {
		//				return nil
		//			}

		//			return err
		//		}

		return nil
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
	if cluster.IsErrContainerNotFound(err) {
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
		return errors.WithStack(ctx.Err())
	}

	logrus.WithFields(logrus.Fields{
		"Unit":   u.u.Name,
		"Engine": u.u.EngineID,
	}).Info("remove container volumes...")

	engine := u.getEngine()
	if engine == nil {
		for i := range lvs {
			_, err := u.cluster.RemoveVolumes(lvs[i].Name)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	} else {
		for i := range lvs {
			err := engine.RemoveVolume(lvs[i].Name)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return u.recycleSANSource(lvs)
}

func (u unit) recycleSANSource(lvs []database.Volume) error {
	san := make([]database.Volume, 0, len(lvs))

	for i := range lvs {
		if lvs[i].DriverType == storage.SANStore {
			san = append(san, lvs[i])
		}
	}

	if len(san) == 0 {
		return nil
	}

	actor := alloc.NewAllocator(u.uo, u.cluster)

	return actor.RecycleResource(nil, san)
}

// ContainerExec returns the container exec command result,message of exec print into write
func (u unit) ContainerExec(ctx context.Context, cmd []string, detach bool, w io.Writer) (types.ContainerExecInspect, error) {
	if len(cmd) == 0 {
		return types.ContainerExecInspect{}, nil
	}
	c := u.getContainer()
	if c == nil {
		return types.ContainerExecInspect{}, errors.WithStack(newContainerError(u.u.Name, notFound))
	}

	if !c.Info.State.Running {
		return types.ContainerExecInspect{}, errors.WithStack(newContainerError(u.u.Name, notRunning))
	}

	return c.Exec(ctx, cmd, detach, w)
}

func (u unit) containerExec(ctx context.Context, cmd []string, detach bool) (types.ContainerExecInspect, error) {
	if len(cmd) == 0 {
		return types.ContainerExecInspect{}, nil
	}
	c := u.getContainer()
	if c == nil {
		return types.ContainerExecInspect{}, errors.WithStack(newContainerError(u.u.Name, notFound))
	}

	if !c.Info.State.Running {
		return types.ContainerExecInspect{}, errors.WithStack(newContainerError(u.u.Name, notRunning))
	}

	return c.Exec(ctx, cmd, detach, nil)
}

// updateServiceConfig update new service config context,backup file first.
func (u unit) updateServiceConfig(ctx context.Context, path, context string, backup bool) error {
	if backup {
		err := u.backupServiceConfig(ctx, path)
		if err != nil {
			return err
		}
	}

	cmd := []string{"/bin/sh", "-c", fmt.Sprintf(`echo "%s" > %s`, context, path)}

	inspect, err := u.containerExec(ctx, cmd, false)
	if err != nil {
		logrus.WithField("Container", u.u.Name).Errorf("update config file %s,%#v,%+v", path, inspect, err)
	}

	return err
}

func (u unit) backupServiceConfig(ctx context.Context, path string) error {
	ext := filepath.Ext(path)
	dst := strings.Replace(path, ext, "-"+time.Now().Format("2006-01-02T15:04:05")+ext, 1)

	cmd := []string{"cp", path, dst}

	inspect, err := u.containerExec(ctx, cmd, false)
	if err != nil {
		logrus.WithField("Container", u.u.Name).Errorf("backup config file %s-->%s,%#v,%+v", path, dst, inspect, err)
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

func (u unit) startNetworking(ctx context.Context, host string, tlsConfig *tls.Config) error {
	ips, err := u.uo.ListIPByUnitID(u.u.ID)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return nil
	}

	sys, err := u.uo.GetSysConfig()
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(host, strconv.Itoa(sys.Ports.SwarmAgent))

	for i := range ips {
		err := alloc.CreateNetworkDevice(ctx, addr, u.u.ContainerID, ips[i], tlsConfig)
		if err != nil {
			return err
		}
	}

	return nil
}
