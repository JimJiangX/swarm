package cluster

import (
	"github.com/docker/swarm/garden/utils"
)

// UsedCpus returns the sum of CPUs reserved by containers.
func (e *Engine) UsedCpus() int64 {
	var r int64
	e.RLock()
	for _, c := range e.containers {

		if c.Config.HostConfig.CpusetCpus == "" {
			r += c.Config.HostConfig.CPUShares
		} else {

			n, err := utils.CountCPU(c.Config.HostConfig.CpusetCpus)
			if err != nil {
				// TODO:
			}

			r += n
		}
	}
	e.RUnlock()
	return r
}
