package swarm

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
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
	DockerNodesKVPath  = "docker/swarm/nodes"
	DatacenterID       = 0
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

	sysConfig          *database.Configurations
	cron               *crontab.Cron // crontab tasks
	consulClient       *consulapi.Client
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
	DockerNodesKVPath = parseKVuri(uri)

	cluster, ok := cli.(*Cluster)
	if !ok {
		logrus.Fatal("cluster.Cluster Prototype is not *swarm.Cluster")
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
			HostAddress = protoAddrParts[1]
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

func (gd *Gardener) SetParams(sys *database.Configurations) error {
	gd.Lock()
	defer gd.Unlock()

	_, clients := pingConsul(HostAddress, sys)
	gd.consulClient = clients[0]

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
		BackupDir:  req.BackupDir,
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

	endpoints, clients := pingConsul(HostAddress, &config)
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

func pingHorus(addr string) error {
	_, err := http.Post("http://"+addr+"/v1/ping", "", nil)
	return err
}
