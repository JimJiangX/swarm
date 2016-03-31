package swarm

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	crontab "gopkg.in/robfig/cron.v2"
)

type Region struct {
	*Cluster

	// addition by fugr
	cron     *crontab.Cron // crontab tasks
	cronJobs map[crontab.EntryID]*database.BackupStrategy

	datacenters []*Datacenter
	networkings []*Networking
	services    []*Service
	stores      []store.Store

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
		cronJobs:           make(map[crontab.EntryID]*database.BackupStrategy),
		datacenters:        make([]*Datacenter, 0, 50),
		networkings:        make([]*Networking, 0, 50),
		services:           make([]*Service, 0, 100),
		stores:             make([]store.Store, 0, 50),
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

func (r *Region) RegisterBackupStrategy(id crontab.EntryID, strategy *database.BackupStrategy) error {
	entry := r.cron.Entry(id)
	if entry.ID != id {
		return fmt.Errorf("Cron Job Not Found,%d", id)
	}

	r.Lock()
	r.cronJobs[id] = strategy
	r.Unlock()

	return nil
}

func (r *Region) RemoveCronJob(strategyID string) error {
	r.Lock()

	for key, val := range r.cronJobs {
		if val.ID == strategyID {
			r.cron.Remove(key)
			return nil
		}
	}

	r.Unlock()

	return nil
}
