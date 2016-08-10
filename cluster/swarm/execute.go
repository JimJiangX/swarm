package swarm

import (
	"fmt"
	"runtime/debug"
	"sync/atomic"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/cluster"
	"github.com/pkg/errors"
)

func (gd *Gardener) serviceExecute(svc *Service) (err error) {
	svc.Lock()

	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Recover From Panic:%v", r)
			debug.PrintStack()

			err = fmt.Errorf("serviceExecute:Recover From Panic,%v", r)
		}

		if err == nil {
			atomic.StoreInt64(&svc.Status, statusServiceNoContent)

			svc.Unlock()
			return
		}

		logrus.Errorf("Service %s Execute Failed,%s", svc.Name, err)

		gd.scheduler.Lock()
		for _, u := range svc.units {
			if u == nil {
				continue
			}
			delete(gd.pendingContainers, u.ID)
		}
		gd.scheduler.Unlock()

		svc.Unlock()
	}()

	err = svc.statusCAS(statusServiceAllocting, statusServiceCreating)
	if err != nil {
		return err
	}

	logrus.Debugf("Execute Service %s", svc.Name)

	err = gd.createServiceContainers(svc)
	if err != nil {
		logrus.Errorf("%s create Service Containers Error:%s", svc.Name, err)

		return err
	}

	err = gd.initAndStartService(svc)
	if err != nil {
		logrus.Errorf("%s init And Start Service Error:%s", svc.Name, err)

		return err
	}

	logrus.Debugf("Service %s Created,running...", svc.Name)

	return nil
}

func (gd *Gardener) createServiceContainers(svc *Service) (err error) {
	defer func() {
		if err != nil {
			logrus.Errorf("%s create Service Containers error:%s", svc.Name, err)

			atomic.StoreInt64(&svc.Status, statusServiceCreateFailed)
		}
	}()

	funcs := make([]func() error, len(svc.units))
	for i := range svc.units {
		if svc.units[i] == nil {
			logrus.Warning("%s:nil pointer in service units", svc.Name)
			continue
		}

		swarmID := svc.units[i].ID

		funcs[i] = func() error {
			err := svc.createPendingContainer(gd, swarmID)
			if err != nil {
				logrus.Errorf("swarmID %s,Error:%s", swarmID, err)
			}

			return err
		}
	}

	err = GoConcurrency(funcs)
	if err != nil {
		logrus.Errorf("Create Service %s Containers Error:%s", svc.Name, err)
	}

	return err
}

func (svc *Service) createPendingContainer(gd *Gardener, swarmID string) (err error) {
	if swarmID == "" {
		return fmt.Errorf("The swarmID is Null")
	}

	if svc.authConfig == nil {
		svc.authConfig, err = gd.RegistryAuthConfig()
		if err != nil {
			return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
		}
	}

	u, err := svc.getUnit(swarmID)
	if err != nil || u == nil {
		return fmt.Errorf("%s:get unit err:%v", svc.Name, err)
	}

	atomic.StoreInt64(&u.Unit.Status, statusUnitCreating)

	logrus.Debug("[MG]create container")
	container, err := gd.createContainerInPending(swarmID, svc.authConfig)
	if err != nil {
		atomic.StoreInt64(&u.Unit.Status, statusUnitCreateFailed)
		u.LatestError = err.Error()

		u.saveToDisk()

		return fmt.Errorf("Container Create Failed %s,%s", swarmID, err)
	}

	logrus.Debug("container created:", container.ID)

	u.container = container
	u.config = container.Config
	u.Unit.ContainerID = container.ID

	atomic.StoreInt64(&u.Unit.Status, statusUnitCreated)
	u.Unit.LatestError = ""

	if err := u.saveToDisk(); err != nil {
		return errors.Errorf("Update Unit %s value error:%s,Unit:%+v", u.Name, err, u.Unit)
	}

	err = saveContainerToConsul(container)
	if err != nil {
		logrus.Errorf("Save Container To Consul error:%s", err)
	}

	return nil
}

// createContainerInPending create new container into the cluster.
func (gd *Gardener) createContainerInPending(swarmID string, authConfig *types.AuthConfig) (*cluster.Container, error) {
	gd.scheduler.Lock()
	pending, ok := gd.pendingContainers[swarmID]
	gd.scheduler.Unlock()

	if !ok || pending == nil || pending.Engine == nil || pending.Config == nil {
		return nil, fmt.Errorf("Swarm ID Not Found in pendingContainers,%s", swarmID)
	}

	engine := pending.Engine
	container, err := engine.CreateContainer(pending.Config, pending.Name, true, authConfig)
	if err != nil {
		logrus.WithFields(logrus.Fields{"Name": "Swarm"}).Warnf("Failed to create container: %s", err)
	}

	if err == nil && container != nil {
		gd.scheduler.Lock()
		delete(gd.pendingContainers, swarmID)
		gd.scheduler.Unlock()
	}

	return container, err
}

func (gd *Gardener) initAndStartService(svc *Service) (err error) {
	sys, err := gd.SystemConfig()
	if err != nil {
		logrus.Errorf("Query Database Error:%s", err)
		return nil
	}

	err = svc.statusCAS(statusServiceCreating, statusServiceStarting)
	if err != nil {
		return err
	}
	defer func() {
		for _, u := range svc.units {
			u.saveToDisk()
		}

		if err != nil {
			// mark failed
			atomic.StoreInt64(&svc.Status, statusServiceStartFailed)
		} else {
			atomic.StoreInt64(&svc.Status, statusServiceNoContent)
		}
	}()

	logrus.Debug("[MG]starting Containers")
	if err := svc.startContainers(); err != nil {
		return err
	}

	logrus.Debug("[MG]copy Service Config")
	if err := svc.copyServiceConfig(); err != nil {
		return err
	}

	logrus.Debug("[MG]init & Start Service")
	if err := svc.initService(); err != nil {
		return err
	}

	logrus.Debug("[MG]initTopology")
	if err := svc.initTopology(); err != nil {
		return err
	}

	logrus.Debug("[MG]registerServices")
	if err := svc.registerServices(sys.ConsulConfig); err != nil {
		return err
	}

	logrus.Debug("[MG]registerToHorus")

	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	err = svc.registerToHorus(horus, sys.MonitorUsername, sys.MonitorPassword, sys.HorusAgentPort)
	if err != nil {
		logrus.Warnf("register To Horus Error:%s", err)
	}

	return nil
}
