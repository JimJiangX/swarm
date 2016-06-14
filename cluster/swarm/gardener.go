package swarm

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	kvdiscovery "github.com/docker/docker/pkg/discovery/kv"
	"github.com/docker/engine-api/types"
	kvstore "github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	consulapi "github.com/hashicorp/consul/api"
	crontab "gopkg.in/robfig/cron.v2"
)

var leaderElectionPath = "docker/swarm/leader"

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
}

func UpdateleaderElectionPath(path string) {
	leaderElectionPath = path
}

type Gardener struct {
	*Cluster

	// addition by fugr
	host   string
	kvPath string

	cron               *crontab.Cron // crontab tasks
	consulClient       *consulapi.Client
	kvClient           kvstore.Store
	registryAuthConfig *types.AuthConfig
	cronJobs           map[crontab.EntryID]*serviceBackup

	datacenters []*Datacenter
	networkings []*Networking
	services    []*Service
	stores      []store.Store

	serviceSchedulerCh chan *Service // scheduler service units
	serviceExecuteCh   chan *Service // run service containers
}

// NewGardener is exported
func NewGardener(cli cluster.Cluster, uri string, hosts []string) (*Gardener, error) {
	logrus.WithFields(logrus.Fields{"name": "swarm"}).Debug("Initializing Gardener")
	cluster, ok := cli.(*Cluster)
	if !ok {
		logrus.Fatal("cluster.Cluster Prototype is not *swarm.Cluster")
	}

	gd := &Gardener{
		Cluster:            cluster,
		kvPath:             parseKVuri(uri),
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
		logrus.Warning("kvDiscovery is only supported with consul, etcd and zookeeper discovery.")
	}

	// query consul config from DB
	sysConfig, err := database.GetSystemConfig()
	if err != nil {
		logrus.Error("Get System Config Error,%s", err)
	} else {
		DatacenterID = sysConfig.DCID
		err = gd.SetParams(*sysConfig)
		if err != nil {
			logrus.Error(err)
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

func (gd *Gardener) RegistryAuthConfig() (*types.AuthConfig, error) {
	if gd.registryAuthConfig != nil {
		return gd.registryAuthConfig, nil
	}

	c, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	return &types.AuthConfig{
		Username:      c.Username,
		Password:      c.Password,
		Email:         c.Email,
		RegistryToken: c.Registry.Token,
	}, nil
}

func (gd *Gardener) KvPath() string {
	gd.RLock()
	path := gd.kvPath
	gd.RUnlock()

	return path
}

func (gd *Gardener) setConsulClient(client *consulapi.Client) {
	gd.Lock()
	gd.consulClient = client
	gd.Unlock()
}

func (gd *Gardener) consulAPIClient(full bool) (*consulapi.Client, error) {
	if !full {
		gd.RLock()
		if gd.consulClient != nil {
			if _, err := gd.consulClient.Status().Leader(); err == nil {
				gd.RUnlock()
				return gd.consulClient, nil
			}
		}
		gd.RUnlock()
	}

	config, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	clients, err := config.GetConsulClient()
	if err != nil {
		return nil, err
	}
	for i := range clients {
		_, err := clients[i].Status().Leader()
		if err == nil {
			gd.setConsulClient(clients[i])

			return clients[i], nil
		}
	}

	return nil, fmt.Errorf("Not Found Alive Consul Server %s:%d", config.ConsulIPs, config.ConsulPort)
}

func (gd *Gardener) SetParams(sys database.Configurations) error {
	gd.Lock()
	defer gd.Unlock()

	if sys.Retry > 0 && gd.Cluster.createRetry == 0 {
		gd.Cluster.createRetry = sys.Retry
	}

	if sys.PluginPort > 0 {
		pluginPort = sys.PluginPort
	}

	gd.registryAuthConfig = &types.AuthConfig{
		Username:      sys.Registry.Username,
		Password:      sys.Registry.Password,
		Email:         sys.Registry.Email,
		RegistryToken: sys.Registry.Token,
	}

	endpoints, dc, token, wait := sys.GetConsulConfig()

	if len(endpoints) == 0 {
		return fmt.Errorf("Consul Config Settings Error")
	}

	config := consulapi.Config{
		Address:    endpoints[0],
		Datacenter: dc,
		WaitTime:   time.Duration(wait) * time.Second,
		Token:      token,
	}

	consulClient, err := consulapi.NewClient(&config)
	if err != nil {
		return err
	}
	gd.consulClient = consulClient

	options := &kvstore.Config{
		TLS:               gd.TLSConfig(),
		ConnectionTimeout: config.WaitTime,
	}

	if gd.kvClient == nil {
		gd.kvClient, err = consul.New(endpoints, options)
		if err != nil {
			logrus.Error("Initializing kvStore,consul Config Error,%s", err)
			return err
		}
	}

	return nil
}
