package garden

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
	"github.com/docker/swarm/scheduler"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

type notFound struct {
	key  string
	elem string
}

func (nf notFound) Error() string {
	if nf.key != "" {
		return fmt.Sprintf("%s not found %s", nf.elem, nf.key)
	}

	return fmt.Sprintf("%s not found", nf.elem)
}

func newNotFound(elem, key string) notFound {
	return notFound{
		elem: elem,
		key:  key,
	}
}

type Garden struct {
	sync.Mutex
	ormer        database.Ormer
	kvClient     kvstore.Client
	pluginClient pluginapi.PluginAPI

	allocator allocator
	cluster.Cluster
	scheduler  *scheduler.Scheduler
	tlsConfig  *tls.Config
	authConfig *types.AuthConfig
}

func NewGarden(kvc kvstore.Client, cl cluster.Cluster,
	scheduler *scheduler.Scheduler, ormer database.Ormer,
	allocator allocator, pClient pluginapi.PluginAPI, tlsConfig *tls.Config) *Garden {
	return &Garden{
		// Mutex:       &scheduler.Mutex,
		kvClient:     kvc,
		allocator:    allocator,
		Cluster:      cl,
		ormer:        ormer,
		pluginClient: pClient,
		tlsConfig:    tlsConfig,
	}
}

func (gd *Garden) KVClient() kvstore.Client {
	return gd.kvClient
}

func (gd *Garden) AuthConfig() (*types.AuthConfig, error) {
	return gd.ormer.GetAuthConfig()
}

func (gd *Garden) Ormer() database.Ormer {
	return gd.ormer
}

func (gd *Garden) PluginClient() pluginapi.PluginAPI {
	return gd.pluginClient
}

func (gd *Garden) TLSConfig() *tls.Config {
	return gd.tlsConfig
}

// Register set Garden,returns a error if has registered in database
func (gd *Garden) Register(req structs.RegisterDC) error {
	sys, err := gd.ormer.GetSysConfig()
	if err == nil {
		return errors.Errorf("DC has registered,dc=%d", sys.ID)
	}

	//	err = validGardenRegister(&req)
	//	if err != nil {
	//		return err
	//	}

	config := database.SysConfig{
		ID:        req.ID,
		BackupDir: req.BackupDir,
		Retry:     req.Retry,
		Ports: database.Ports{
			Docker:     req.DockerPort,
			Plugin:     req.PluginPort,
			SwarmAgent: req.SwarmAgentPort,
		},
		ConsulConfig: database.ConsulConfig{
			ConsulIPs:        req.Consul.ConsulIPs,
			ConsulPort:       req.Consul.ConsulPort,
			ConsulDatacenter: req.Consul.ConsulDatacenter,
			ConsulToken:      req.Consul.ConsulToken,
			ConsulWaitTime:   req.Consul.ConsulWaitTime,
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
			CACert:     req.Registry.CACert,
		},
		SSHDeliver: database.SSHDeliver{
			SourceDir:       req.SSHDeliver.SourceDir,
			CACertName:      req.SSHDeliver.CACertName,
			Destination:     req.SSHDeliver.Destination,
			InitScriptName:  req.SSHDeliver.InitScriptName,
			CleanScriptName: req.SSHDeliver.CleanScriptName,
		},
		//		Users: database.Users{
		//			MonitorUsername:     req.Users.MonitorUsername,
		//			MonitorPassword:     req.Users.MonitorPassword,
		//			ReplicationUsername: req.Users.ReplicationUsername,
		//			ReplicationPassword: req.Users.ReplicationPassword,
		//			ApplicationUsername: req.Users.ApplicationUsername,
		//			ApplicationPassword: req.Users.ApplicationPassword,
		//			DBAUsername:         req.Users.DBAUsername,
		//			DBAPassword:         req.Users.DBAPassword,
		//			DBUsername:          req.Users.DBUsername,
		//			DBPassword:          req.Users.DBPassword,
		//		},
	}

	kvc := gd.KVClient()
	if kvc == nil {
		ips, dc, token, wt := config.GetConsulConfig()
		cfg := &consulapi.Config{
			Address:    ips[0],
			Datacenter: dc,
			WaitTime:   time.Second * time.Duration(wt),
			Token:      token,
		}

		kvc, err = kvstore.MakeClient(cfg, "garden", strconv.Itoa(config.ConsulPort), gd.tlsConfig)
		if err != nil {
			return err
		}

		gd.Lock()
		gd.kvClient = kvc
		gd.Unlock()
	}

	err = pingHorus(kvc)
	if err != nil {
		logrus.Warnf("%+v", err)
	}

	//	err = nfsSetting(config.NFSOption)
	//	if err != nil {
	//		logrus.Errorf("%+v", err)

	//		return err
	//	}

	err = gd.ormer.InsertSysConfig(config)

	return err
}

func pingHorus(kvc kvstore.Client) error {
	addr, err := kvc.GetHorusAddr()
	if err != nil {
		return err
	}

	resp, err := http.Post("http://"+addr+"/v1/ping", "", nil)
	if err == nil {
		if resp != nil && resp.Body != nil {
			io.CopyN(ioutil.Discard, resp.Body, 512)

			resp.Body.Close()
		}

		return nil
	}

	return errors.Wrap(err, "ping Horus")
}

func validGardenRegister(req *structs.RegisterDC) error {
	buf := bytes.NewBuffer(nil)

	if buf.Len() == 0 {
		return nil
	}

	return errors.New(buf.String())
}
