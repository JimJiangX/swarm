package gardener

import (
	"allocation/scheduler"
	"crypto/tls"

	"github.com/docker/docker/pkg/discovery"
)

type Region struct {
	*Cluster

	// addition by fugr
	cron          *crontab.Cron // crontab tasks
	allocatedPort int64

	datacenters []*Datacenter
	networkings []*Networking
	services    []*Service

	serviceSchedulerCh chan *Service // scheduler service units
	serviceExecuteCh   chan *Service // run service containers
}

// NewCluster is exported
func NewRegion(scheduler *scheduler.Scheduler, TLSConfig *tls.Config, discovery discovery.Backend, options cluster.DriverOpts, engineOptions *cluster.EngineOpts) (cluster.Region, error) {
	log.WithFields(log.Fields{"name": "swarm"}).Debug("Initializing cluster")

	cl, err := NewCluster(scheduler, TLSConfig, discovery, options, engineOptions)
	if err != nil {
		return err
	}

	region := &Region{
		Cluster:            cl,
		cron:               crontab.New(),
		datacenters:        make([]*Datacenter, 0, 50),
		networkings:        make([]*Networking, 0, 50),
		services:           make([]*Service, 0, 100),
		serviceSchedulerCh: make(chan *Service, 100),
		serviceExecuteCh:   make(chan *Service, 100),
	}

	region.cron.Start()
	go region.ServiceScheduler()
	go region.ServiceExecute()

	return region, nil
}
