package gardener

import (
	"fmt"
	"regexp"
	"sync/atomic"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/samalba/dockerclient"
)

func (c *Cluster) ServiceToExecute(svc *Service) {
	c.serviceExecuteCh <- svc
}

func (c *Cluster) ServiceExecute() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Recover From Panic:%v,Error:%s", r, err)
		}
	}()

	for {
		svc := <-c.serviceExecuteCh

		if !atomic.CompareAndSwapInt64(&svc.Status, 0, 1) {
			continue
		}

		svc.Lock()

		for swarmID, pending := range svc.pendingContainers {
			// resource allocation

			// resource prepared

			// create container

			container, err := c.CreateContainer_UPM(swarmID, pending, svc.authConfig)
			if err != nil {
				goto failure
			}

			err = c.StartContainer(container)
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
func (c *Cluster) CreateContainer_UPM(swarmID string, pending *pendingContainer, authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
	container, err := c.createContainer_UPM(swarmID, pending, authConfig)

	if err != nil {
		var retries int64
		config := pending.Config
		//  fails with image not found, then try to reschedule with image affinity
		bImageNotFoundError, _ := regexp.MatchString(`image \S* not found`, err.Error())
		if bImageNotFoundError && !config.HaveNodeConstraint() {
			// Check if the image exists in the cluster
			// If exists, retry with a image affinity
			if c.Image(config.Image) != nil {
				container, err = c.createContainer_UPM(swarmID, pending, authConfig)
				retries++
			}
		}

		for ; retries < c.createRetry && err != nil; retries++ {
			log.WithFields(log.Fields{"Name": "Swarm"}).Warnf("Failed to create container: %s, retrying", err)
			container, err = c.createContainer_UPM(swarmID, pending, authConfig)
		}
	}

	return container, err
}

func (c *Cluster) createContainer_UPM(swarmID string, pending *pendingContainer, authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
	c.scheduler.Lock()
	config := pending.Config

	id := config.SwarmID()
	if swarmID == "" || id != swarmID {
		swarmID = id

		if swarmID == "" {
			c.scheduler.Unlock()
			return nil, fmt.Errorf("Conflict: The swarmID is Null,assign %s to a container", pending.Name)
		}
	}

	// Ensure the name is available
	if !c.checkNameUniqueness(pending.Name) {
		c.scheduler.Unlock()
		return nil, fmt.Errorf("Conflict: The name %s is already assigned. You have to delete (or rename) that container to be able to assign %s to a container again.", pending.Name, pending.Name)
	}

	if network := c.Networks().Get(config.HostConfig.NetworkMode); network != nil && network.Scope == "local" {
		if !config.HaveNodeConstraint() {
			config.AddConstraint("node==~" + network.Engine.Name)
		}
		config.HostConfig.NetworkMode = network.Name
	}

	container, err := pending.Engine.Create(config, pending.Name, true, authConfig)

	c.scheduler.Lock()
	delete(c.pendingContainers, swarmID)
	c.scheduler.Unlock()

	return container, err
}
