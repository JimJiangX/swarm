package swarm

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	kvdiscovery "github.com/docker/docker/pkg/discovery/kv"
	kvstore "github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/samalba/dockerclient"
	crontab "gopkg.in/robfig/cron.v2"
)

type Gardener struct {
	*Cluster

	// addition by fugr
	host         string
	cron         *crontab.Cron // crontab tasks
	consulClient *consulapi.Client
	kvClient     kvstore.Store
	cronJobs     map[crontab.EntryID]*serviceBackup

	datacenters []*Datacenter
	networkings []*Networking
	services    []*Service
	stores      []store.Store

	serviceSchedulerCh chan *Service // scheduler service units
	serviceExecuteCh   chan *Service // run service containers
}

// NewGardener is exported
func NewGardener(cli cluster.Cluster, hosts []string) (*Gardener, error) {
	log.WithFields(log.Fields{"name": "swarm"}).Debug("Initializing Gardener")
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

	kvDiscovery, ok := cluster.discovery.(*kvdiscovery.Discovery)
	if ok {
		gd.kvClient = kvDiscovery.Store()
	} else {
		log.Warning("kvDiscovery is only supported with consul, etcd and zookeeper discovery.")
	}

	// query consul config from DB
	endpoints, dc, token, wait, err := database.GetConsulConfig()
	if err != nil {
		log.Fatalf("Initializing kvStore,DB Query Error,%s", err)
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("Consul Config Settings Error")
	}

	config := consulapi.Config{
		Address:    endpoints[0],
		Datacenter: dc,
		WaitTime:   time.Duration(wait) * time.Second,
		Token:      token,
	}

	gd.consulClient, err = consulapi.NewClient(&config)
	if err != nil {
		return nil, err
	}

	options := &kvstore.Config{
		TLS:               gd.TLSConfig(),
		ConnectionTimeout: config.WaitTime,
	}

	if gd.kvClient == nil {
		gd.kvClient, err = consul.New(endpoints, options)
		if err != nil {
			log.Fatalf("Initializing kvStore,consul Config Error,%s", err)
		}
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

func (gd *Gardener) TLSConfig() *tls.Config {
	return gd.Cluster.TLSConfig
}

func (gd *Gardener) RegistryAuthConfig() (*dockerclient.AuthConfig, error) {
	c, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	return &dockerclient.AuthConfig{
		Username:      c.Username,
		Password:      c.Password,
		Email:         c.Email,
		RegistryToken: c.RegistryToken,
	}, nil
}
