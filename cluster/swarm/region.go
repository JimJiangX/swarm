package swarm

import (
	"crypto/tls"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/discovery"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/scheduler"
	crontab "gopkg.in/robfig/cron.v2"
)

type Region struct {
	*Cluster

	// addition by fugr
	cron *crontab.Cron // crontab tasks

	datacenters []*Datacenter
	networkings []*Networking
	services    []*Service

	serviceSchedulerCh chan *Service // scheduler service units
	serviceExecuteCh   chan *Service // run service containers
}

// NewRegion is exported
func NewRegion(scheduler *scheduler.Scheduler, TLSConfig *tls.Config, discovery discovery.Backend, options cluster.DriverOpts, engineOptions *cluster.EngineOpts) (*Region, error) {
	log.WithFields(log.Fields{"name": "swarm"}).Debug("Initializing cluster")

	// NewCluster,copy from cluster.go
	cluster := &Cluster{
		eventHandlers:     cluster.NewEventHandlers(),
		engines:           make(map[string]*cluster.Engine),
		pendingEngines:    make(map[string]*cluster.Engine),
		scheduler:         scheduler,
		TLSConfig:         TLSConfig,
		discovery:         discovery,
		pendingContainers: make(map[string]*pendingContainer),
		overcommitRatio:   0.05,
		engineOpts:        engineOptions,
		createRetry:       0,
	}

	if val, ok := options.Float("swarm.overcommit", ""); ok {
		cluster.overcommitRatio = val
	}

	if val, ok := options.Int("swarm.createretry", ""); ok {
		if val < 0 {
			log.Fatalf("swarm.createretry=%d is invalid", val)
		}
		cluster.createRetry = val
	}

	discoveryCh, errCh := cluster.discovery.Watch(nil)
	go cluster.monitorDiscovery(discoveryCh, errCh)
	go cluster.monitorPendingEngines()

	log.WithFields(log.Fields{"name": "swarm"}).Debug("Initializing Region")

	region := &Region{
		Cluster:            cluster,
		cron:               crontab.New(),
		datacenters:        make([]*Datacenter, 0, 50),
		networkings:        make([]*Networking, 0, 50),
		services:           make([]*Service, 0, 100),
		serviceSchedulerCh: make(chan *Service, 100),
		serviceExecuteCh:   make(chan *Service, 100),
	}

	region.cron.Start()
	go region.serviceScheduler()
	go region.serviceExecute()

	return region, nil
}

func (r *Region) generateUUID(length int) string {
	for {
		id := generateUUID(length)
		if r.Container(id) == nil {
			return id
		}
	}
}
