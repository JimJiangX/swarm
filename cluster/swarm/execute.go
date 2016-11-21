package swarm

import (
	"runtime/debug"
	"sync/atomic"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/cluster"
	"github.com/pkg/errors"
)

func (gd *Gardener) serviceExecute(svc *Service) (err error) {
	entry := logrus.WithField("Service", svc.Name)

	defer func() {
		if r := recover(); r != nil {
			debug.PrintStack()

			err = errors.Errorf("serviceExecute:recover from panic,%v", r)
		}

		if err == nil {
			return
		}

		entry.WithError(err).Errorf("Service Execute failed")

		gd.scheduler.Lock()
		for _, u := range svc.units {
			if u == nil {
				continue
			}
			delete(gd.pendingContainers, u.ID)
		}
		gd.scheduler.Unlock()
	}()

	err = svc.statusLock.SetStatus(statusServiceContainerCreating)
	if err != nil {
		return err
	}

	err = gd.createServiceContainers(svc)
	if err != nil {
		entry.WithError(err).Error("create containers")
		return err
	}

	err = gd.initAndStartService(svc)
	if err != nil {
		entry.WithError(err).Error("Init and Start Service")

		return err
	}

	entry.Debug("Service Created,running...")

	return nil
}

func (gd *Gardener) createServiceContainers(svc *Service) (err error) {
	defer func() {
		if err != nil {
			_err := svc.statusLock.SetStatus(statusServiceContainerCreateFailed)
			if _err != nil {
				logrus.Errorf("%s create Service Containers failed,%+v", svc.Name, _err)
			}
		}
	}()

	funcs := make([]func() error, 0, len(svc.units))
	for i := range svc.units {
		if svc.units[i] == nil {
			logrus.WithField("Service", svc.Name).Warn("nil pointer in service units")
			continue
		}

		swarmID := svc.units[i].ID

		funcs = append(funcs, func() error {
			err := svc.createPendingContainer(gd, swarmID)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"Service": svc.Name,
					"swarmID": swarmID,
					"Error":   err.Error(),
				}).Error("create containers")
			}

			return err
		})
	}

	err = GoConcurrency(funcs)
	if err != nil {
		logrus.Errorf("Create Service %s Containers Error:%s", svc.Name, err)
	}

	return err
}

func (svc *Service) createPendingContainer(gd *Gardener, swarmID string) (err error) {
	if swarmID == "" {
		return errors.New("the swarmID is required")
	}

	if svc.authConfig == nil {
		svc.authConfig, err = gd.registryAuthConfig()
		if err != nil {
			return errors.Wrap(err, "get registry AuthConfig")
		}
	}

	u, err := svc.getUnit(swarmID)
	if err != nil {
		return err
	}

	atomic.StoreInt64(&u.Unit.Status, statusUnitCreating)

	container, err := gd.createContainerInPending(swarmID, svc.authConfig)
	if err != nil {
		atomic.StoreInt64(&u.Unit.Status, statusUnitCreateFailed)
		u.LatestError = err.Error()

		_err := u.saveToDisk()
		if _err != nil {
			logrus.WithField("Service", svc.Name).Errorf("%s,update Unit:%+v", err, _err)
		}

		return err
	}

	logrus.Debug("container created:", container.ID)

	u.container = container
	u.config = container.Config
	u.Unit.ContainerID = container.ID

	atomic.StoreInt64(&u.Unit.Status, statusUnitCreated)
	u.Unit.LatestError = ""

	if err := u.saveToDisk(); err != nil {
		return err
	}

	err = saveContainerToConsul(container)
	if err != nil {
		logrus.Errorf("save container to Consul:%+v", err)
	}

	return nil
}

// createContainerInPending create new container into the cluster.
func (gd *Gardener) createContainerInPending(swarmID string, authConfig *types.AuthConfig) (*cluster.Container, error) {
	gd.scheduler.Lock()
	pending, ok := gd.pendingContainers[swarmID]
	gd.scheduler.Unlock()

	if !ok || pending == nil || pending.Engine == nil || pending.Config == nil {
		return nil, errors.New("swarmID not found in pendingContainers")
	}

	engine := pending.Engine
	container, err := engine.CreateContainer(pending.Config, pending.Name, true, authConfig)
	engine.CheckConnectionErr(err)
	if err != nil {
		logrus.WithFields(logrus.Fields{"Name": "Swarm"}).Warnf("Failed to create container: %s", err)
	}

	if err == nil && container != nil {
		gd.scheduler.Lock()
		delete(gd.pendingContainers, swarmID)
		gd.scheduler.Unlock()
	}

	return container, errors.Wrap(err, "create container")
}

func (gd *Gardener) initAndStartService(svc *Service) (err error) {
	err = svc.statusLock.SetStatus(statusServiceStarting)
	if err != nil {
		return err
	}

	defer func() {
		for _, u := range svc.units {
			_err := u.saveToDisk()
			if _err != nil {
				logrus.WithField("Unit", u.Name).Errorf("%+v", err)
			}
		}

		state := statusServiceStarted
		if err != nil {
			state = statusServiceStartFailed
		}

		_err := svc.statusLock.SetStatus(state)
		if _err != nil {
			logrus.WithField("Service", svc.Name).Errorf("%+v", err)
		}
	}()

	mon, err := svc.getUserByRole(_User_Monitor_Role)
	if err != nil {
		return err
	}

	sys, err := gd.systemConfig()
	if err != nil {
		return err
	}

	logrus.Debug("starting Containers")
	if err := svc.startContainers(); err != nil {
		return err
	}

	logrus.Debug("copy Service Config")
	if err := svc.copyServiceConfig(); err != nil {
		return err
	}

	logrus.Debug("init & Start Service")
	if err := svc.initService(); err != nil {
		return err
	}

	logrus.Debug("register Services")
	if err := svc.registerServices(); err != nil {
		return err
	}

	logrus.Debug("registerToHorus")
	err = svc.registerToHorus(mon.Username, mon.Password, sys.HorusAgentPort)
	if err != nil {
		logrus.Warnf("register To Horus Error:%s", err)
	}

	logrus.Debug("init Topology")
	if err := svc.initTopology(); err != nil {
		return err
	}

	return nil
}
