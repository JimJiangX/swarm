package swarm

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	"github.com/pkg/errors"
	crontab "gopkg.in/robfig/cron.v2"
)

var (
	leaderElectionPath = "docker/swarm/leader"
	hostAddress        = "127.0.0.1"
	httpPort           = "4000"
	dockerNodesKVPath  = "docker/swarm/nodes"
	// DatacenterID is Gardener registered ID
	DatacenterID = 0
)

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
}

// UpdateleaderElectionPath set leaderElectionPath,called in mainage.go L143
func UpdateleaderElectionPath(path string) {
	leaderElectionPath = path
}

// Gardener is exported
type Gardener struct {
	*Cluster

	// added by fugr
	sysConfig           *database.Configurations
	cron                *crontab.Cron // crontab tasks
	registry_authConfig *types.AuthConfig
	cronJobs            map[crontab.EntryID]*serviceBackup

	// datacenters []*Datacenter
	networkings []*Networking
	//	services    []*Service
}

// NewGardener is exported
func NewGardener(cli cluster.Cluster, uri string, hosts []string) (*Gardener, error) {
	logrus.WithFields(logrus.Fields{"name": "swarm"}).Debug("Initializing Gardener")

	dockerNodesKVPath = parseKVuri(uri)

	cluster, ok := cli.(*Cluster)
	if !ok {
		logrus.Fatal("cluster.Cluster Prototype is not *swarm.Cluster")
	}

	gd := &Gardener{
		Cluster:  cluster,
		cron:     crontab.New(),
		cronJobs: make(map[crontab.EntryID]*serviceBackup),
		//		datacenters: make([]*Datacenter, 0, 50),
		networkings: make([]*Networking, 0, 50),
		//	services:    make([]*Service, 0, 100),
	}

	for _, host := range hosts {
		protoAddrParts := strings.SplitN(host, "://", 2)
		if len(protoAddrParts) == 1 {
			protoAddrParts = append([]string{"tcp"}, protoAddrParts...)
		}
		if protoAddrParts[0] == "tcp" {
			ip, port, err := net.SplitHostPort(protoAddrParts[1])
			if err != nil {
				logrus.Errorf("%s SplitHostPort error,%s", protoAddrParts[1], err)
				return nil, err
			}

			hostAddress = ip
			httpPort = port
			break
		}
	}

	// query consul config from DB
	sysConfig, err := database.GetSystemConfig()
	if err != nil {
		logrus.Errorf("Get System Config Error,%s", err)
	} else {
		err = gd.setParams(sysConfig)
		if err != nil {
			logrus.Error(err)
		}
	}

	// go gd.syncNodeWithEngine()

	return gd, nil
}

// StartCron starts cron
func (gd *Gardener) StartCron() error {
	if gd.cron == nil {
		gd.cron = crontab.New()
		gd.cronJobs = make(map[crontab.EntryID]*serviceBackup)
	}
	defer gd.cron.Start()

	out, err := database.ListBackupStrategy(true)
	if err != nil {
		logrus.Errorf("start cron:%+v", err)
		return err
	}

	for i := range out {
		svc, err := gd.GetService(out[i].ServiceID)
		if err == nil && svc != nil {
			svc.Lock()
			svc.backup = &out[i]
			svc.Unlock()

			bs := newBackupJob(svc)
			gd.registerBackupStrategy(bs)
		}
	}

	return nil
}

// StopCron stop cron
func (gd *Gardener) StopCron() {
	if gd.cron == nil {
		return
	}

	gd.cron.Stop()
}

// syncNodeWithEngine synchronize Node.engine if Node exist
// engine from new Engine register to Cluster
//func (gd *Gardener) syncNodeWithEngine() {
//	gd.Lock()
//	if gd.Cluster.pendingEngineCh == nil {
//		gd.Cluster.pendingEngineCh = make(chan *cluster.Engine, 50)
//	}
//	gd.Unlock()

//	for {
//		engine := <-gd.Cluster.pendingEngineCh
//		if engine == nil || !engine.IsHealthy() {
//			continue
//		}

//		nodeTab, err := database.GetNode(engine.ID)
//		if err != nil {
//			logrus.WithField("Engine", engine.Addr).WithError(err).Warn("sync Node with Engine")

//			nodeTab, err = database.GetNodeByAddr(engine.Addr)
//			if err != nil {
//				logrus.WithField("Engine", engine.Addr).Warn(err)
//				continue
//			}
//			if nodeTab.Status < statusNodeEnable {
//				continue
//			}
//		}

//				var dc *Datacenter

//				gd.RLock()
//				for i := range gd.datacenters {
//					if gd.datacenters[i].ID == nodeTab.ClusterID {
//						dc = gd.datacenters[i]
//						break
//					}
//				}
//				gd.RUnlock()

//				if dc == nil {
//		dc, err := gd.Datacenter(nodeTab.ClusterID)
//		if err != nil {
//			logrus.WithField("Engine", engine.Addr).Warn(err)
//		}

//				node, err := dc.GetNode(nodeTab.ID)
//				if err == nil {
//					if node.engine == nil ||
//						(!node.engine.IsHealthy() && node.engine.Addr == engine.Addr) {

//						node.engine = engine
//					}

//				} else {
//					node, err = gd.reloadNode(nodeTab)
//					if err != nil {
//						continue
//					}
//					if dc != nil {
//						dc.Lock()
//						dc.nodes = append(dc.nodes, node)
//						dc.Unlock()
//					}
//				}

//				err = gd.reloadServiceByEngine(engine.ID)
//				if err != nil {
//					logrus.WithField("Engine", engine.Addr).WithError(err).Warn("sync Node with Engine,reload Services on engine")
//				}
//	}
//}

//func (gd *Gardener) reloadServiceByEngine(engineID string) error {
//	units, err := database.ListUnitByEngine(engineID)
//	if err != nil {
//		return err
//	}

//	list := make([]string, 0, len(units))

//	for i := range units {
//		exist := false
//		for j := range list {
//			if list[j] == units[i].ServiceID {
//				exist = true
//				break
//			}
//		}

//		if !exist {
//			list = append(list, units[i].ServiceID)
//		}
//	}

//	for i := range list {
//		var svc *Service

//		gd.RLock()
//		for s := range gd.services {
//			if gd.services[s].ID == list[i] {
//				svc = gd.services[i]
//				break
//			}
//		}
//		gd.RUnlock()

//		if svc != nil {
//			svc.Lock()
//		}
//		_, err := gd.reloadService(list[i])
//		if err != nil {
//			logrus.WithField("Service", list[i]).WithError(err).Error("reload service")
//		}
//		if svc != nil {
//			svc.Unlock()
//		}
//	}

//	return nil
//}

func (gd *Gardener) generateUUID(length int) string {
	for {
		id := utils.GenerateUUID(length)
		if gd.Container(id) == nil {
			return id
		}
	}
}

func (gd *Gardener) registryAuthConfig() (*types.AuthConfig, error) {
	if gd.registry_authConfig != nil {
		return gd.registry_authConfig, nil
	}

	c, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	gd.registry_authConfig = &types.AuthConfig{
		Username:      c.Username,
		Password:      c.Password,
		Email:         c.Email,
		RegistryToken: c.Registry.Token,
	}

	return gd.registry_authConfig, nil
}

func (gd *Gardener) setParams(sys *database.Configurations) error {
	gd.Lock()
	defer gd.Unlock()

	err := setConsulClient(&sys.ConsulConfig)
	if err != nil {
		return err
	}

	DatacenterID = sys.ID

	if sys.Retry > 0 && gd.Cluster.createRetry == 0 {
		gd.Cluster.createRetry = sys.Retry
	}

	if sys.PluginPort > 0 {
		pluginPort = sys.PluginPort
	}

	gd.registry_authConfig = &types.AuthConfig{
		Username:      sys.Registry.Username,
		Password:      sys.Registry.Password,
		Email:         sys.Registry.Email,
		RegistryToken: sys.Registry.Token,
	}

	gd.sysConfig = sys

	return nil
}

// Register set Gardener,returns a error if has registered in database
func (gd *Gardener) Register(req structs.RegisterGardener) error {
	sys, err := database.GetSystemConfig()
	if err == nil {
		return errors.Errorf("DC has registered,dc=%d", sys.ID)
	}

	err = validGardenerRegister(&req)
	if err != nil {
		return err
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
			HorusAgentPort: req.Horus.HorusAgentPort,
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
			CheckUsername:       req.Users.CheckUsername,
			CheckPassword:       req.Users.CheckPassword,
			ReadOnlyUsername:    req.Users.ReadOnlyUsername,
			ReadOnlyPassword:    req.Users.ReadOnlyPassword,
		},
	}

	err = pingHorus()
	if err != nil {
		logrus.Warnf("%+v", err)
	}

	//	err = nfsSetting(config.NFSOption)
	//	if err != nil {
	//		logrus.Errorf("%+v", err)

	//		return err
	//	}

	_, err = config.Insert()
	if err != nil {
		logrus.Errorf("%+v", err)

		return err
	}

	err = gd.setParams(&config)
	if err != nil {
		logrus.Errorf("%+v", err)
	}

	return err
}

func (gd *Gardener) systemConfig() (database.Configurations, error) {
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

	err = gd.setParams(sys)
	if err != nil {
		return *sys, err
	}

	return *sys, nil
}

func nfsSetting(option database.NFSOption) error {
	if option.Addr == "" || option.Dir == "" || option.MountDir == "" {
		logrus.Warnf("NFS option:%v", option)
	}

	_, err := os.Stat(option.MountDir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(option.MountDir, os.ModePerm)
		if err != nil {
			return errors.Wrap(err, "MountDir is required,MkdirAll")
		}
	}

	// mount -t nfs -o nolock 192.168.2.181:/NASBACKUP /NASBACKUP
	// option addr:dir mount_dir
	script := fmt.Sprintf("mount -t nfs -o %s %s:%s %s",
		option.MountOptions, option.Addr, option.Dir, option.MountDir)

	cmd, err := utils.ExecScript(script)
	if err != nil {
		return errors.Wrapf(err, "mount NFS:%s", cmd.Args)
	}

	out, err := cmd.Output()
	if err != nil {
		return errors.Wrapf(err, "exec cmd:%s,output:%s", cmd.Args, out)
	}

	return nil
}

func pingHorus() error {
	addr, err := getHorusFromConsul()
	if err != nil {
		return err
	}

	resp, err := http.Post("http://"+addr+"/v1/ping", "", nil)
	if err == nil {
		ensureReaderClosed(resp)
		return nil
	}

	return errors.Wrap(err, "ping Horus")
}
