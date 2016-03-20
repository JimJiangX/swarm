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

		for swarmID, pending := range svc.pendingContainers {

			// create container

			container, err := region.CreateContainer(swarmID, pending, svc.authConfig)
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

// CreateContainer aka schedule a brand new container into the cluster.
func (region *Region) CreateContainer(swarmID string, pending *pendingContainer, authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
	container, err := region.createContainer(swarmID, pending, authConfig)

	if err != nil {
		var retries int64
		config := pending.Config
		//  fails with image not found, then try to reschedule with image affinity
		bImageNotFoundError, _ := regexp.MatchString(`image \S* not found`, err.Error())
		if bImageNotFoundError && !config.HaveNodeConstraint() {
			// Check if the image exists in the cluster
			// If exists, retry with a image affinity
			if region.Image(config.Image) != nil {
				container, err = region.createContainer(swarmID, pending, authConfig)
				retries++
			}
		}

		for ; retries < region.createRetry && err != nil; retries++ {
			log.WithFields(log.Fields{"Name": "Swarm"}).Warnf("Failed to create container: %s, retrying", err)
			container, err = region.createContainer(swarmID, pending, authConfig)
		}
	}

	return container, err
}

func (region *Region) createContainer(swarmID string, pending *pendingContainer, authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
	region.scheduler.Lock()
	config := pending.Config

	id := config.SwarmID()
	if swarmID == "" || id != swarmID {
		swarmID = id

		if swarmID == "" {
			region.scheduler.Unlock()
			return nil, fmt.Errorf("Conflict: The swarmID is Null,assign %s to a container", pending.Name)
		}
	}

	// Ensure the name is available
	if !region.checkNameUniqueness(pending.Name) {
		region.scheduler.Unlock()
		return nil, fmt.Errorf("Conflict: The name %s is already assigned. You have to delete (or rename) that container to be able to assign %s to a container again.", pending.Name, pending.Name)
	}

	if network := region.Networks().Get(config.HostConfig.NetworkMode); network != nil && network.Scope == "local" {
		if !config.HaveNodeConstraint() {
			config.AddConstraint("node==~" + network.Engine.Name)
		}
		config.HostConfig.NetworkMode = network.Name
	}

	container, err := pending.Engine.Create(config, pending.Name, true, authConfig)

	region.scheduler.Lock()
	delete(region.pendingContainers, swarmID)
	region.scheduler.Unlock()

	return container, err
}
