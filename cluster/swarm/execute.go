package swarm

import (
	"fmt"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
)

func (gd *Gardener) ServiceToExecute(svc *Service) {
	gd.serviceExecuteCh <- svc
}

func (gd *Gardener) serviceExecute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v", r)
		}

		log.Fatalf("Service Execute Exit,%s", err)
	}()

	for {
		svc := <-gd.serviceExecuteCh

		if !atomic.CompareAndSwapInt64(&svc.Status, _StatusServiceAlloction, _StatusServiceCreating) {
			continue
		}

		svc.Lock()

		// step := int64(0)
		var taskErr error

		for _, pending := range svc.pendingContainers {

			u, err := svc.getUnit(pending.Name)
			if err != nil {
				svc.Unlock()
				taskErr = err

				goto failure
			}
			atomic.StoreUint32(&u.Status, _StatusUnitCreating)

			err = u.prepareCreateContainer()
			if err != nil {
				svc.Unlock()
				taskErr = fmt.Errorf("Prepare Networking or Volume Failed,%s", err)

				goto failure
			}

			// create container
			container, err := gd.createContainerInPending(pending.Config, pending.Name, svc.authConfig)
			if err != nil {
				svc.Unlock()
				taskErr = fmt.Errorf("Container Create Failed %s,%s", pending.Name, err)

				goto failure
			}

			u.container = container

			if err := u.saveToDisk(); err != nil {

			}
		}

		svc.pendingContainers = nil

		svc.Unlock()

		err = svc.StartContainers()
		if err != nil {
			taskErr = err

			goto failure
		}

		err = svc.CopyServiceConfig()
		if err != nil {
			taskErr = err

			goto failure
		}

		err = svc.StartService()
		if err != nil {
			taskErr = err

			goto failure
		}

		err = svc.CreateUsers()
		if err != nil {
			taskErr = err

			goto failure
		}

		err = svc.InitTopology()
		if err != nil {
			taskErr = err

			goto failure
		}

		err = svc.RegisterServices()
		if err != nil {
			taskErr = err

			goto failure
		}

		database.TxSetServiceStatus(&svc.Service, svc.task,
			_StatusServiceNoContent, _StatusTaskDone, time.Now(), "")

		continue

	failure:

		//TODO:error handler

		database.TxSetServiceStatus(&svc.Service, svc.task, svc.Status, _StatusTaskFailed, time.Now(), taskErr.Error())
	}

	return err
}

// createContainerInPending create new container into the cluster.
func (gd *Gardener) createContainerInPending(config *cluster.ContainerConfig, name string, authConfig *types.AuthConfig) (*cluster.Container, error) {
	// Ensure the name is available
	if !gd.checkNameUniqueness(name) {
		return nil, fmt.Errorf("Conflict: The name %s is already assigned. You have to delete (or rename) that container to be able to assign %s to a container again.", name, name)
	}

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
