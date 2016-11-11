package cluster

import (
	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/swarmclient"
	"github.com/docker/swarm/utils"
)

// UsedCpus returns the sum of CPUs reserved by containers.
// Copy From engine.go L949,
// Parse Config.HostConfig.CpusetCpus to get usdCPU.
func (e *Engine) UsedCpus() int64 {
	var r int64
	e.RLock()
	for _, c := range e.containers {
		cpuset := c.Config.HostConfig.CpusetCpus
		ncpu, err := utils.GetCPUNum(cpuset)
		if err != nil {
			log.WithFields(log.Fields{
				"ID":         c.ID,
				"CpusetCpus": cpuset,
			}).Error("Parse CpusetCpus Error", err)
		}

		r += ncpu
	}

	e.RUnlock()
	return r
}

// ContainerAPIClient returns Engine SwarmAPIClient
func (e *Engine) SwarmAPIClient() swarmclient.SwarmAPIClient {
	if e == nil {
		return nil
	}

	e.Lock()
	client := e.apiClient
	e.Unlock()

	return client
}
