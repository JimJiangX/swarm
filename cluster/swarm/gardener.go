package swarm

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	kvdiscovery "github.com/docker/docker/pkg/discovery/kv"
	"github.com/docker/engine-api/types"
	kvstore "github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	"github.com/docker/swarm/utils"
	consulapi "github.com/hashicorp/consul/api"
	crontab "gopkg.in/robfig/cron.v2"
)

var (
	leaderElectionPath = "docker/swarm/leader"
	HostAddress        = "127.0.0.1"
)

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

	sysConfig          *database.Configurations
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

	for _, host := range hosts {
		protoAddrParts := strings.SplitN(host, "://", 2)
		if len(protoAddrParts) == 1 {
			protoAddrParts = append([]string{"tcp"}, protoAddrParts...)
		}
		if protoAddrParts[0] == "tcp" {
			gd.host = protoAddrParts[1]
			HostAddress = gd.host
			break
		}
	}

	// query consul config from DB
	sysConfig, err := database.GetSystemConfig()
	if err != nil {
		logrus.Error("Get System Config Error,%s", err)
	} else {
		err = gd.SetParams(sysConfig)
		if err != nil {
			logrus.Error(err)
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

func (gd *Gardener) ConsulAPIClient(full bool) (*consulapi.Client, error) {
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

	sys, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	clients, err := sys.GetConsulClient()
	if err != nil {
		logrus.Error(err)
	}
	config := consulapi.Config{
		Address:    fmt.Sprintf("%s:%d", gd.host, sys.ConsulPort),
		Datacenter: sys.ConsulDatacenter,
		WaitTime:   time.Duration(sys.ConsulWaitTime) * time.Second,
		Token:      sys.ConsulToken,
	}

	consulClient, err := consulapi.NewClient(&config)
	if err != nil {
		logrus.Warnf("%s ,%v", err, config)
	} else {
		clients = append(clients, consulClient)
	}

	for i := len(clients) - 1; i >= 0; i-- {
		if clients[i] == nil {
			continue
		}
		_, err := clients[i].Status().Leader()
		if err == nil {
			gd.setConsulClient(clients[i])

			return clients[i], nil
		}
	}

	return nil, fmt.Errorf("Not Found Alive Consul Server %s:%d", sys.ConsulIPs, sys.ConsulPort)
}

func (gd *Gardener) SetParams(sys *database.Configurations) error {
	gd.Lock()
	defer gd.Unlock()

	endpoints, clients := pingConsul(gd.host, sys)
	gd.consulClient = clients[0]

	options := &kvstore.Config{
		TLS:               gd.TLSConfig(),
		ConnectionTimeout: time.Duration(sys.ConsulWaitTime) * time.Second,
	}

	for _, endpoint := range endpoints {
		client, err := consul.New([]string{endpoint}, options)
		if err != nil {
			logrus.Error("Initializing kvStore,consul Config %s Error,%s", endpoint, err)
		} else {
			gd.kvClient = client
			break
		}
	}

	DatacenterID = sys.ID

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

	gd.sysConfig = sys

	return nil
}

func RegisterDatacenter(gd *Gardener, req structs.RegisterDatacenter) error {
	sys, err := database.GetSystemConfig()
	if err == nil {
		return fmt.Errorf("DC Has Registered,dc=%d", sys.ID)
	}

	config := database.Configurations{
		ID:         req.ID,
		DockerPort: req.DockerPort,
		PluginPort: req.PluginPort,
		Retry:      req.Retry,
		NFSOption: database.NFSOption{
			Addr:         req.NFS.Addr,
			Dir:          req.NFS.Dir,
			MountDir:     req.NFS.MountDir,
			MountOptions: req.NFS.MountOptions,
		},
		ConsulConfig: database.ConsulConfig{
			ConsulIPs:        req.Consul.ConsulIPs,
			ConsulPort:       req.Consul.ConsulPort,
			ConsulDatacenter: req.Consul.ConsulDatacenter,
			ConsulToken:      req.Consul.ConsulToken,
			ConsulWaitTime:   req.Consul.ConsulWaitTime,
		},
		HorusConfig: database.HorusConfig{
			HorusServerIP:   req.Horus.HorusServerIP,
			HorusServerPort: req.Horus.HorusServerPort,
			HorusAgentPort:  req.Horus.HorusAgentPort,
			HorusEventIP:    req.Horus.HorusEventIP,
			HorusEventPort:  req.Horus.HorusEventPort,
		},
		Registry: database.Registry{
			OsUsername: req.Registry.OsUsername,
			OsPassword: req.Registry.OsPassword,
			Domain:     req.Registry.Domain,
			Address:    req.Registry.Address,
			Port:       req.Registry.Port,
			Username:   req.Registry.Username,
			Password:   req.Registry.Password,
			Email:      req.Registry.Email,
			Token:      req.Registry.Token,
			CA_CRT:     req.Registry.CA_CRT,
		},
		SSHDeliver: database.SSHDeliver{
			SourceDir:       req.SSHDeliver.SourceDir,
			CA_CRT_Name:     req.SSHDeliver.CA_CRT_Name,
			Destination:     req.SSHDeliver.Destination,
			InitScriptName:  req.SSHDeliver.InitScriptName,
			CleanScriptName: req.SSHDeliver.CleanScriptName,
		},
		Users: database.Users{
			MonitorUsername:     req.Users.MonitorUsername,
			MonitorPassword:     req.Users.MonitorPassword,
			ReplicationUsername: req.Users.ReplicationUsername,
			ReplicationPassword: req.Users.ReplicationPassword,
			ApplicationUsername: req.Users.ApplicationUsername,
			ApplicationPassword: req.Users.ApplicationPassword,
			DBAUsername:         req.Users.DBAUsername,
			DBAPassword:         req.Users.DBAPassword,
			DBUsername:          req.Users.DBUsername,
			DBPassword:          req.Users.DBPassword,
		},
	}

	horus := fmt.Sprintf("%s:%d", config.HorusServerIP, config.HorusServerPort)
	err = pingHorus(horus)
	if err != nil {
		logrus.Errorf("Ping Horus %s error,%s", horus, err)
		return err
	}

	endpoints, clients := pingConsul(gd.host, &config)
	if len(endpoints) == 0 || len(clients) == 0 {
		return fmt.Errorf("cannot connect consul")
	}

	err = nfsSetting(config.NFSOption)
	if err != nil {
		logrus.Error(err)
		return err
	}

	_, err = config.Insert()
	if err != nil {
		return err
	}

	err = gd.SetParams(&config)
	if err != nil {
		logrus.Error(err)
	}

	return err
}

func (gd *Gardener) SystemConfig() (database.Configurations, error) {
	gd.RLock()
	config := gd.sysConfig
	gd.RUnlock()

	if config != nil {
		return *config, nil
	}

	sys, err := database.GetSystemConfig()
	if err != nil || sys == nil {
		return database.Configurations{}, err
	}

	err = gd.SetParams(sys)
	if err != nil {
		return database.Configurations{}, err
	}

	return *sys, nil
}

func nfsSetting(option database.NFSOption) error {
	if option.Addr == "" || option.Dir == "" || option.MountDir == "" {
		logrus.Warnf("NFS Option:%v", option)
		return nil
	}
	_, err := os.Stat(option.MountDir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(option.MountDir, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// mount -t nfs -o nolock 192.168.2.181:/NASBACKUP /NASBACKUP
	// option addr:dir mount_dir
	script := fmt.Sprintf("mount -t nfs -o %s %s:%s %s",
		option.MountOptions, option.Addr, option.Dir, option.MountDir)

	cmd, err := utils.ExecScript(script)
	if err != nil {
		return err
	}

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%v,%s", err, string(out))
	}

	return nil
}

func pingConsul(host string, sys *database.Configurations) ([]string, []*consulapi.Client) {
	endpoints, dc, token, wait := sys.GetConsulConfig()
	port := strconv.Itoa(sys.ConsulPort)
	endpoints = append(endpoints, host+":"+port)

	endpoints[0], endpoints[len(endpoints)-1] = endpoints[len(endpoints)-1], endpoints[0]

	peers := make([]string, 0, len(endpoints))
	clients := make([]*consulapi.Client, 0, len(endpoints))

	for _, endpoint := range endpoints {
		config := consulapi.Config{
			Address:    endpoint,
			Datacenter: dc,
			WaitTime:   time.Duration(wait) * time.Second,
			Token:      token,
		}

		client, err := consulapi.NewClient(&config)
		if err != nil {
			logrus.Warnf("consul config illegal，%v", config)
			continue
		}

		servers, err := client.Status().Peers()
		if err != nil {
			continue
		}

		addrs := make([]string, 0, len(servers))
		for n := range servers {
			parts := strings.Split(servers[n], ":")
			if len(parts) == 2 {
				servers[n] = parts[0] + ":" + port
				addrs = append(addrs, parts[0])
			}
		}
		sys.ConsulIPs = strings.Join(addrs, ",")

		exist := false
		for n := range servers {
			if endpoint == servers[n] {
				exist = true
				break
			}
		}
		if !exist {
			peers = append(peers, endpoint)
			peers = append(peers, servers...)

			clients = append(clients, client)

		} else {
			peers = servers
		}

		for i := range servers {
			config := consulapi.Config{
				Address:    servers[i],
				Datacenter: dc,
				WaitTime:   time.Duration(wait) * time.Second,
				Token:      token,
			}

			client, err := consulapi.NewClient(&config)
			if err != nil {
				logrus.Warnf("consul config illegal，%v", config)
				continue
			}
			clients = append(clients, client)
		}
	}

	return peers, clients
}

func pingHorus(addr string) error {
	_, err := http.Post("http://"+addr+"/v1/ping", "", nil)
	return err
}
