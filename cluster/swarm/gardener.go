package swarm

import (
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	crontab "gopkg.in/robfig/cron.v2"
)

type Gardener struct {
	*Cluster

	// addition by fugr
	host     string
	cron     *crontab.Cron // crontab tasks
	cronJobs map[crontab.EntryID]*serviceBackup

	datacenters []*Datacenter
	networkings []*Networking
	services    []*Service
	stores      []store.Store

	serviceSchedulerCh chan *Service // scheduler service units
	serviceExecuteCh   chan *Service // run service containers
}

// NewGardener is exported
func NewGardener(cli cluster.Cluster, hosts []string) (*Gardener, error) {
	log.WithFields(log.Fields{"name": "swarm"}).Debug("Initializing Region")
	cluster, ok := cli.(*Cluster)
	if !ok {
		log.Fatal("cluster.Cluster Prototype is not *swarm.Cluster")
	}

	gd := &Gardener{
		Cluster:            cluster,
		cron:               crontab.New(),
		cronJobs:           make(map[crontab.EntryID]*serviceBackup),
		datacenters:        make([]*Datacenter, 0, 50),
		networkings:        make([]*Networking, 0, 50),
		services:           make([]*Service, 0, 100),
		stores:             make([]store.Store, 0, 50),
		serviceSchedulerCh: make(chan *Service, 100),
		serviceExecuteCh:   make(chan *Service, 100),
	}

	for _, host := range hosts {
		protoAddrParts := strings.SplitN(host, "://", 2)
		if len(protoAddrParts) == 1 {
			protoAddrParts = append([]string{"tcp"}, protoAddrParts...)
		}
		if protoAddrParts[0] == "tcp" {
			gd.host = protoAddrParts[1]
			break
		}
	}

	gd.cron.Start()
	go gd.serviceScheduler()
	go gd.serviceExecute()

	return gd, nil
}

func (gd *Gardener) generateUUID(length int) string {
	for {
		id := utils.GenerateUUID(length)
		if gd.Container(id) == nil {
			return id
		}
	}
}
