package swarm

import (
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

		if !atomic.CompareAndSwapInt64(&svc.Status, _StatusServiceAlloction, _StatusServiceCreating) {
			continue
		}
		sysConfig, err := database.GetSystemConfig()
		if err != nil {
			log.Errorf("Query Database Error:%s", err.Error())
			continue
		}
		horusServerAddr := fmt.Sprintf("%s:%d", sysConfig.HorusServerIP, sysConfig.HorusServerPort)

		consulClient, err := gd.consulAPIClient(false)
		if err != nil {
			log.Error("consul client is nil", err)
			continue
		}

		svc.Lock()
		log.Debugf("[**MG**]serviceExecute: get server: %v", svc)
		// step := int64(0)
		var taskErr error

		err = createContainerInPending(gd, svc)
		if err != nil {
			svc.Unlock()
			taskErr = err
			goto failure
		}

		svc.pendingContainers = nil

		svc.Unlock()

		log.Debug("[mg]starting containers")
		err = svc.StartContainers()
		if err != nil {
			taskErr = err
			goto failure
		}

		log.Debug("[mg]CopyServiceConfig")
		err = svc.CopyServiceConfig()
		if err != nil {
			taskErr = err
			goto failure
		}

		log.Debug("[mg]InitService")
		err = svc.InitService()
		if err != nil {
			taskErr = err
			goto failure
		}

		log.Debug("[mg]CreateUsers")
		err = svc.CreateUsers()
		if err != nil {
			taskErr = err
			goto failure
		}
		log.Debug("[mg]InitTopology")
		err = svc.InitTopology()
		if err != nil {
			taskErr = err
			goto failure
		}

		log.Debug("[mg]RegisterServices")
		err = svc.RegisterServices(consulClient)
		if err != nil {
			taskErr = err
			goto failure
		}

		log.Debug("[mg]TxSetServiceStatus")

		database.TxSetServiceStatus(&svc.Service, svc.task,
			_StatusServiceNoContent, _StatusTaskDone, time.Now(), "")

		log.Debug("[mg]RegisterToHorus")

		err = svc.RegisterToHorus(horusServerAddr,
			sysConfig.MonUsername, sysConfig.MonPassword, sysConfig.HorusAgentPort)
		if err != nil {
			taskErr = err
			goto failure
		}

		log.Debug("[mg]exec done")
		continue

	failure:

		//TODO:error handler
		log.Error("Exec Error:%v", taskErr)
		database.TxSetServiceStatus(&svc.Service, svc.task, svc.Status, _StatusTaskFailed, time.Now(), taskErr.Error())
	}

	return err
}

func createContainerInPending(gd *Gardener, svc *Service) error {
	for _, pending := range svc.pendingContainers {
		log.Debugf("[mg]svc.getUnit :%s", pending)
		u, err := svc.getUnit(pending.Name)
		if err != nil || u == nil {
			log.Errorf("%s:getunit err:%v", pending.Name, err)
			return err
		}
		log.Debugf("[mg]the unit:%v", u)
		atomic.StoreUint32(&u.Status, _StatusUnitCreating)

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

		log.Debug("[mg]start prepareCreateContainer")
		err = u.prepareCreateContainer()
		if err != nil {
			err = fmt.Errorf("%s:Prepare Networking or Volume Failed,%s", pending.Name, err)
			log.Error(err.Error())
			return err
		}

		log.Debug("[mg]create container")
		// create container
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

	return nil
}

func (gd *Gardener) SaveContainerToConsul(container *cluster.Container) error {
	client, err := gd.consulAPIClient(true)
	if err != nil {
		return err
	}

	buf, err := json.Marshal(container)
	if err != nil {
		return err
	}

	pair := &consulapi.KVPair{
		Key:   "/DBAAS/Conatainers/" + container.Info.Name,
		Value: buf,
	}
	_, err = client.KV().Put(pair, nil)

	return err
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
