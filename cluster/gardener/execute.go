package gardener

import (
	"fmt"
	"regexp"
	"sync/atomic"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/samalba/dockerclient"
)

func (r *Region) ServiceToExecute(svc *Service) {
	r.serviceExecuteCh <- svc
}

func (region *Region) ServiceExecute() (err error) {
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
			container, err := region.CreateContainerInPending(pending.Config, "", svc.authConfig)
			if err != nil {
				goto failure
			}

			err = region.StartContainer(container)
			if err != nil {
				goto failure
			}

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

// CreateContainerInPending aka schedule a brand new container into the cluster.
func (r *Region) CreateContainerInPending(config *cluster.ContainerConfig, name string, authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
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

	if network := r.Networks().Get(config.HostConfig.NetworkMode); network != nil && network.Scope == "local" {
		if !config.HaveNodeConstraint() {
			config.AddConstraint("node==~" + network.Engine.Name)
		}
		config.HostConfig.NetworkMode = network.Name
	}

	engine := pending.Engine
	container, err := engine.Create(config, name, true, authConfig)

	if err != nil {
		var retries int64
		//  fails with image not found, then try to reschedule with image affinity
		bImageNotFoundError, _ := regexp.MatchString(`image \S* not found`, err.Error())
		if bImageNotFoundError && !config.HaveNodeConstraint() {
			// Check if the image exists in the cluster
			// If exists, retry with a image affinity
			if r.Image(config.Image) != nil {
				container, err = engine.Create(config, name, true, authConfig)
				retries++
			}
		}

		for ; retries < r.createRetry && err != nil; retries++ {
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
