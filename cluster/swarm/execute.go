package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	consulapi "github.com/hashicorp/consul/api"
)

func (gd *Gardener) ServiceToExecute(svc *Service) {
	gd.serviceExecuteCh <- svc
}

func (gd *Gardener) serviceExecute() error {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Recover From Panic:%v", r)
		}
		debug.PrintStack()

		logrus.Fatal("Service Execute Exit")
	}()

	for {
		svc := <-gd.serviceExecuteCh
		svc.Lock()
		err := svc.statusCAS(_StatusServiceAlloction, _StatusServiceCreating)
		if err != nil {
			logrus.Error(err)
			continue
		}

		logrus.Debugf("[MG]Execute Service:%v", svc)

		err = gd.createServiceContainers(svc)
		if err != nil {
			logrus.Errorf("%s create Service Containers Error:%s", svc.Name, err)
			goto failure
		}

		err = gd.initAndStartService(svc)
		if err != nil {
			logrus.Errorf("%s init And Start Service Error:%s", svc.Name, err)
			goto failure
		}

		logrus.Debug("[MG]TxSetServiceStatus")
		err = database.TxSetServiceStatus(&svc.Service, svc.task, _StatusServiceNoContent, _StatusTaskDone, time.Now(), "")
		if err != nil {
			logrus.Errorf("%s TxSetServiceStatus Error:%s", svc.Name, err)
			goto failure
		}
		svc.Unlock()

		logrus.Debugf("[MG]Service %s Created,running...", svc.Name)

		continue

	failure:

		gd.scheduler.Lock()
		for _, u := range svc.units {
			if u == nil {
				continue
			}
			delete(gd.pendingContainers, u.ID)
		}
		gd.scheduler.Unlock()

		logrus.Errorf("Exec Error:%v", err)
		err = database.TxSetServiceStatus(&svc.Service, svc.task, svc.Status, _StatusTaskFailed, time.Now(), err.Error())
		if err != nil {
			logrus.Errorf("Save Service %s Status Error:%v", svc.Name, err)
		}
		svc.Unlock()

		sys, err := gd.SystemConfig()
		if err != nil {
			continue
		}
		configs := sys.GetConsulConfigs()
		if len(configs) == 0 {
			continue
		}

		horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
		err = svc.Delete(gd, configs[0], horus, true, true, false, 0)
		if err != nil {
			continue
		}
	}
}

func (gd *Gardener) createServiceContainers(svc *Service) (err error) {
	defer func() {
		if err != nil {
			logrus.Errorf("%s create Service Containers error:%s", svc.Name, err)

			atomic.StoreInt64(&svc.Status, _StatusServiceCreateFailed)
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

	logrus.Debug("[MG]create container")
	container, err := gd.createContainerInPending(swarmID, svc.authConfig)
	if err != nil {
		return fmt.Errorf("Container Create Failed %s,%s", swarmID, err)
	}

	logrus.Debug("container created:", container.ID)

	u, err := svc.getUnit(swarmID)
	if err != nil || u == nil {
		return fmt.Errorf("%s:get unit err:%v", svc.Name, err)
	}

	u.container = container
	u.config = container.Config
	u.Unit.ContainerID = container.ID

	if err := u.saveToDisk(); err != nil {
		return fmt.Errorf("update unit %s value error:%s,value:%v", u.Name, err, u)
	}

	err = gd.SaveContainerToConsul(container)
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
	sys, err := database.GetSystemConfig()
	if err != nil {
		logrus.Errorf("Query Database Error:%s", err)
		return nil
	}
	err = svc.statusCAS(_StatusServiceCreating, _StatusServiceStarting)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			// mark failed
			atomic.StoreInt64(&svc.Status, _StatusServiceStartFailed)
		} else {
			atomic.StoreInt64(&svc.Status, _StatusServiceNoContent)
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

func (gd *Gardener) SaveContainerToConsul(container *cluster.Container) error {
	client, err := gd.ConsulAPIClient()
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(nil)
	err = json.NewEncoder(buf).Encode(container)
	if err != nil {
		return err
	}

	pair := &consulapi.KVPair{
		Key:   "DBAAS/Conatainers/" + container.ID,
		Value: buf.Bytes(),
	}
	_, err = client.KV().Put(pair, nil)

	return err
}
