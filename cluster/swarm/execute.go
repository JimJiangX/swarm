package swarm

import (
	"fmt"
	"sync/atomic"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/samalba/dockerclient"
)

func (r *Region) ServiceToExecute(svc *Service) {
	r.serviceExecuteCh <- svc
}

func (region *Region) serviceExecute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%s", r, err)
		}

		log.Fatalf("Service Execute Exit,%s", err)
	}()

	for {
		svc := <-region.serviceExecuteCh

		if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
			continue
		}

		svc.Lock()

		for _, pending := range svc.pendingContainers {

			// create container
			container, err := region.createContainerInPending(pending.Config, pending.Name, svc.authConfig)
			if err != nil {
				goto failure
			}

			unit, err := svc.getUnit(pending.Name)
			if err != nil {
				goto failure
			}

			unit.container = container

		}

		atomic.StoreInt64(&svc.Status, 1)

		svc.Unlock()

		continue

	failure:
		atomic.StoreInt64(&svc.Status, 10)
		svc.Unlock()
	}

	return err
}

// createContainerInPending create new container into the cluster.
func (r *Region) createContainerInPending(config *cluster.ContainerConfig, name string, authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
	// Ensure the name is available
	if !r.checkNameUniqueness(name) {
		return nil, fmt.Errorf("Conflict: The name %s is already assigned. You have to delete (or rename) that container to be able to assign %s to a container again.", name, name)
	}

	swarmID := config.SwarmID()
	if swarmID == "" {
		return nil, fmt.Errorf("Conflict: The swarmID is Null,assign %s to a container", name)
	}

	r.scheduler.Lock()
	pending, ok := r.pendingContainers[swarmID]
	r.scheduler.Unlock()

	if !ok || pending == nil || pending.Engine == nil {
		return nil, fmt.Errorf("Swarm ID Not Found in pendingContainers,%s", swarmID)
	}

	engine := pending.Engine
	container, err := engine.Create(config, name, true, authConfig)

	if err != nil {
		for retries := int64(0); retries < r.createRetry && err != nil; retries++ {
			log.WithFields(log.Fields{"Name": "Swarm"}).Warnf("Failed to create container: %s, retrying", err)
			container, err = engine.Create(config, name, true, authConfig)
		}
	}

	if err == nil && container != nil {
		r.scheduler.Lock()
		delete(r.pendingContainers, swarmID)
		r.scheduler.Unlock()
	}

	return container, err
}
