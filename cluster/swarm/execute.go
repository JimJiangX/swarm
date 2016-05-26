package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
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

func (gd *Gardener) serviceExecute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v", r)
		}
		debug.PrintStack()

		logrus.Fatalf("Service Execute Exit,%s", err)
	}()

	for {
		svc := <-gd.serviceExecuteCh
		svc.Lock()
		err = svc.statusCAS(_StatusServiceAlloction, _StatusServiceCreating)
		if err != nil {
			logrus.Error(err)
			continue
		}

		logrus.Debugf("[mg]Execute Service:%v", svc)

		err = gd.createServiceContainers(svc)
		if err != nil {
			logrus.Errorf("%s create Service Containers Error:%s", svc.Name, err.Error())
			goto failure
		}

		err = gd.initAndStartService(svc)
		if err != nil {
			logrus.Errorf("%s init And Start Service Error:%s", svc.Name, err.Error())
			goto failure
		}

		logrus.Debug("[mg]TxSetServiceStatus")
		err = database.TxSetServiceStatus(&svc.Service, svc.task,
			_StatusServiceNoContent, _StatusTaskDone, time.Now(), "")
		if err != nil {
			logrus.Errorf("%s TxSetServiceStatus Error:%s", svc.Name, err.Error())
			goto failure
		}
		svc.Unlock()
		logrus.Debug("[mg]exec done")
		continue

	failure:

		//TODO:error handler
		logrus.Errorf("Exec Error:%v", err)
		err = database.TxSetServiceStatus(&svc.Service, svc.task, svc.Status, _StatusTaskFailed, time.Now(), err.Error())
		if err != nil {
			logrus.Errorf("Save Service %s Status Error:%v", svc.Name, err)
		}
		svc.Unlock()
	}

	return err
}

func (gd *Gardener) createServiceContainers(svc *Service) (err error) {
	defer func() {
		if err != nil {
			logrus.Error(err)
			atomic.StoreInt64(&svc.Status, _StatusServiceCreateFailed)
		}
	}()

	errCh := make(chan error, len(svc.pendingContainers))
	wg := new(sync.WaitGroup)

	for swarmID := range svc.pendingContainers {
		wg.Add(1)
		go func(gd *Gardener, svc *Service, swarmID string, wg *sync.WaitGroup, ch chan error) {
			defer wg.Done()

			err := svc.createPendingContainer(gd, swarmID)
			if err != nil {
				logrus.Errorf("swarmID %s,Error:%s", swarmID, err)
				ch <- err
			}

		}(gd, svc, swarmID, wg, errCh)
	}

	wg.Wait()
	close(errCh)

	for err1 := range errCh {
		if err1 != nil {
			err = err1
		}
	}

	if err == nil {
		svc.pendingContainers = nil
	}

	return err
}

func (svc *Service) createPendingContainer(gd *Gardener, swarmID string) error {
	if swarmID == "" {
		return fmt.Errorf("The swarmID is Null")
	}
	pending := svc.pendingContainers[swarmID]
	if pending == nil || pending.Config == nil || pending.Engine == nil {
		return fmt.Errorf("pendingContainer or ContainerConfig or Engine is nil")
	}
	logrus.Debugf("[mg]svc.getUnit :%v", pending)

	u, err := svc.getUnit(pending.Name)
	if err != nil || u == nil {
		return fmt.Errorf("%s:getunit err:%v", pending.Name, err)
	}
	logrus.Debugf("[mg]the unit:%v", u)

	logrus.Debugf("[mg]start pull image %s", u.config.Image)
	authConfig, err := gd.RegistryAuthConfig()
	if err != nil {
		return fmt.Errorf("get RegistryAuthConfig Error:%s", err)
	}

	if err := u.pullImage(authConfig); err != nil {
		return fmt.Errorf("pullImage Error:%s", err)
	}

	logrus.Debug("[mg]prepare for Creating Container")
	err = u.prepareCreateContainer()
	if err != nil {
		return fmt.Errorf("%s:Prepare Networking or Volume Failed,%s", pending.Name, err)
	}

	logrus.Debug("[mg]create container")
	container, err := gd.createContainerInPending(swarmID, svc.authConfig)
	if err != nil {
		return fmt.Errorf("Container Create Failed %s,%s", pending.Name, err)
	}
	logrus.Debug("container created:", container)

	u.container = container
	u.config = container.Config
	u.Unit.ContainerID = container.ID

	if err := u.saveToDisk(); err != nil {
		return fmt.Errorf("update unit %s value error:%s,value:%v", u.Name, err, u)
	}

	err = gd.SaveContainerToConsul(container)
	if err != nil {
		logrus.Errorf("Save Container To Consul error:%s", err.Error())
		// return err
	}

	return nil
}

// createContainerInPending create new container into the cluster.
func (gd *Gardener) createContainerInPending(swarmID string, authConfig *types.AuthConfig) (*cluster.Container, error) {
	gd.scheduler.Lock()
	pending, ok := gd.pendingContainers[swarmID]
	gd.scheduler.Unlock()

	if !ok || pending == nil || pending.Engine == nil {
		return nil, fmt.Errorf("Swarm ID Not Found in pendingContainers,%s", swarmID)
	}

	engine := pending.Engine
	container, err := engine.Create(pending.Config, pending.Name, true, authConfig)

	if err != nil {
		for retries := int64(0); retries < gd.createRetry && err != nil; retries++ {
			logrus.WithFields(logrus.Fields{"Name": "Swarm"}).Warnf("Failed to create container: %s, retrying", err)
			container, err = engine.Create(pending.Config, pending.Name, true, authConfig)
		}
	}

	if err == nil && container != nil {
		gd.scheduler.Lock()
		delete(gd.pendingContainers, swarmID)
		gd.scheduler.Unlock()
	}

	return container, err
}

func (gd *Gardener) initAndStartService(svc *Service) error {
	sys, err := database.GetSystemConfig()
	if err != nil {
		logrus.Errorf("Query Database Error:%s", err.Error())
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
	logrus.Debug("[mg]starting Containers")
	if err := svc.startContainers(); err != nil {
		return err
	}

	logrus.Debug("[mg]copy Service Config")
	if err := svc.copyServiceConfig(); err != nil {
		return err
	}

	logrus.Debug("[mg]init & Start Service")
	if err := svc.initService(); err != nil {
		return err
	}

	logrus.Debug("[mg]initTopology")
	if err := svc.initTopology(); err != nil {
		return err
	}

	logrus.Debug("[mg]registerServices")
	if err := svc.registerServices(sys.ConsulConfig); err != nil {
		return err
	}

	logrus.Debug("[mg]registerToHorus")

	horus := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)
	err = svc.registerToHorus(horus, sys.MonitorUsername, sys.MonitorPassword, sys.HorusAgentPort)
	if err != nil {
		logrus.Warnf("register To Horus Error:%s", err.Error())
	}

	return nil
}

func (gd *Gardener) SaveContainerToConsul(container *cluster.Container) error {
	client, err := gd.consulAPIClient(true)
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
