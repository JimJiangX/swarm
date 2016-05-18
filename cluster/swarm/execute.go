package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
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

		log.Fatalf("Service Execute Exit,%s", err)
	}()

	for {
		svc := <-gd.serviceExecuteCh

		err = svc.statusCAS(_StatusServiceAlloction, _StatusServiceCreating)
		if err != nil {
			log.Error(err)
			continue
		}

		log.Debugf("[mg]Execute Service:%v", svc)

		err = gd.createServiceContainers(svc)
		if err != nil {

			goto failure
		}

		err = gd.InitAndStartService(svc)
		if err != nil {
			goto failure
		}

		log.Debug("[mg]TxSetServiceStatus")
		err = database.TxSetServiceStatus(&svc.Service, svc.task,
			_StatusServiceNoContent, _StatusTaskDone, time.Now(), "")

		log.Debug("[mg]exec done")
		continue

	failure:

		//TODO:error handler
		log.Errorf("Exec Error:%v", err)
		err = database.TxSetServiceStatus(&svc.Service, svc.task, svc.Status, _StatusTaskFailed, time.Now(), err.Error())
		if err != nil {
			log.Errorf("Save Service %s Status Error:%v", svc.Name, err)
		}
	}

	return err
}

func (gd *Gardener) createServiceContainers(svc *Service) (err error) {
	svc.Lock()
	defer func() {
		if err != nil {
			log.Error(err)
			atomic.StoreInt64(&svc.Status, _StatusServiceCreateFailed)
		}

		svc.Unlock()
	}()

	for _, pending := range svc.pendingContainers {

		log.Debugf("[mg]svc.getUnit :%s", pending)
		u, err := svc.getUnit(pending.Name)
		if err != nil || u == nil {
			log.Errorf("%s:getunit err:%v", pending.Name, err)
			return err
		}
		log.Debugf("[mg]the unit:%v", u)

		log.Debugf("[mg]start pull image %s", u.config.Image)
		authConfig, err := gd.RegistryAuthConfig()
		if err != nil {
			log.Errorf("get RegistryAuthConfig Error:%s", err.Error())
			return err
		}

		if err := u.pullImage(authConfig); err != nil {
			log.Errorf("pullImage Error:%s", err.Error())
			return err
		}

		log.Debug("[mg]prepare for Creating Container")
		err = u.prepareCreateContainer()
		if err != nil {
			err = fmt.Errorf("%s:Prepare Networking or Volume Failed,%s", pending.Name, err)
			log.Error(err.Error())
			return err
		}

		log.Debug("[mg]create container")
		container, err := gd.createContainerInPending(pending.Config, pending.Name, svc.authConfig)
		if err != nil {
			err = fmt.Errorf("Container Create Failed %s,%s", pending.Name, err)
			log.Error(err.Error())
			return err
		}
		log.Debug("container created:", container)

		u.container = container
		u.Unit.ContainerID = container.ID

		err = gd.SaveContainerToConsul(container)
		if err != nil {
			// return err
		}

		if err := u.saveToDisk(); err != nil {
			// return err
		}
	}

	svc.pendingContainers = nil

	return nil
}

// createContainerInPending create new container into the cluster.
func (gd *Gardener) createContainerInPending(config *cluster.ContainerConfig, name string, authConfig *types.AuthConfig) (*cluster.Container, error) {
	swarmID := config.SwarmID()
	if swarmID == "" {
		return nil, fmt.Errorf("Conflict: The swarmID is Null,assign %s to a container", name)
	}

	gd.scheduler.Lock()
	pending, ok := gd.pendingContainers[swarmID]
	gd.scheduler.Unlock()

	if !ok || pending == nil || pending.Engine == nil {
		return nil, fmt.Errorf("Swarm ID Not Found in pendingContainers,%s", swarmID)
	}

	engine := pending.Engine
	container, err := engine.Create(config, name, true, authConfig)

	if err != nil {
		for retries := int64(0); retries < gd.createRetry && err != nil; retries++ {
			log.WithFields(log.Fields{"Name": "Swarm"}).Warnf("Failed to create container: %s, retrying", err)
			container, err = engine.Create(config, name, true, authConfig)
		}
	}

	if err == nil && container != nil {
		gd.scheduler.Lock()
		delete(gd.pendingContainers, swarmID)
		gd.scheduler.Unlock()
	}

	return container, err
}

func (gd *Gardener) InitAndStartService(svc *Service) error {
	err := svc.statusCAS(_StatusServiceCreating, _StatusServiceStarting)
	if err != nil {
		return err
	}
	svc.Lock()
	defer func() {
		if err != nil {
			// mark failed
			atomic.StoreInt64(&svc.Status, _StatusServiceStartFailed)
		} else {
			atomic.StoreInt64(&svc.Status, _StatusServiceNoContent)
		}
		svc.Unlock()
	}()
	log.Debug("[mg]starting Containers")
	if err := svc.startContainers(); err != nil {
		return err
	}

	log.Debug("[mg]copy Service Config")
	if err := svc.copyServiceConfig(); err != nil {
		return err
	}

	log.Debug("[mg]init & Start Service")
	if err := svc.initService(); err != nil {
		return err
	}

	log.Debug("[mg]create Users")
	if err := svc.createUsers(); err != nil {
		return err
	}

	log.Debug("[mg]initTopology")
	if err := svc.initTopology(); err != nil {
		return err
	}

	consulClient, err := gd.consulAPIClient(false)
	if err != nil {
		log.Error("consul client is nil", err)
		return err
	}
	log.Debug("[mg]registerServices")
	if err := svc.registerServices(consulClient); err != nil {
		return err
	}

	sys, err := database.GetSystemConfig()
	if err != nil {
		log.Errorf("Query Database Error:%s", err.Error())
		return err
	}
	horusServerAddr := fmt.Sprintf("%s:%d", sys.HorusServerIP, sys.HorusServerPort)

	log.Debug("[mg]registerToHorus")
	err = svc.registerToHorus(horusServerAddr, sys.MonUsername, sys.MonPassword, sys.HorusAgentPort)
	if err != nil {
		return err
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
		Key:   "/DBAAS/Conatainers/" + container.ID,
		Value: buf.Bytes(),
	}
	_, err = client.KV().Put(pair, nil)

	return err
}
