package cluster

import (
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/engine-api/client"
)

// UsedCpus returns the sum of CPUs reserved by containers.
// Copy From engine.go L735,
// Parse Config.HostConfig.CpusetCpus to get usdCPU.
func (e *Engine) UsedCpus() int64 {
	var r int64
	e.RLock()
	for _, c := range e.containers {
		cpuset := c.Config.ContainerConfig.HostConfig.CpusetCpus
		ncpu, err := getCPUNum(cpuset)
		if err != nil {
			log.WithFields(log.Fields{
				"ID":         c.Id,
				"CpusetCpus": cpuset,
				"CpuShares":  c.Config.CpuShares,
			}).Error("Parse CpusetCpus Error")
		}

		r += ncpu
	}

	e.RUnlock()
	return r
}

func getCPUNum(val string) (int64, error) {
	cpus, err := parsers.ParseUintList(val)
	if err != nil {
		return 0, err
	}

	ncpu := int64(0)

	for _, v := range cpus {
		if v {
			ncpu++
		}
	}

	return ncpu, nil
}

func (e *Engine) EngineAPIClient() client.APIClient {

	return e.apiClient
}
