package swarm

import (
	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/utils"
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
func NewRegion(cli cluster.Cluster) (*Region, error) {
	log.WithFields(log.Fields{"name": "swarm"}).Debug("Initializing Region")
	cluster, ok := cli.(*Cluster)
	if !ok {
		log.Fatal("cluster.Cluster Prototype is not *swarm.Cluster")
	}

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
		id := utils.GenerateUUID(length)
		if r.Container(id) == nil {
			return id
		}
	}
}

func (r *Region) RegisterBackupStrategy(svc *Service) error {
	return nil
}
