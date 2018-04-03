package garden

import (
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
	"github.com/docker/swarm/garden/resource/alloc"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/tasklock"
	pluginapi "github.com/docker/swarm/plugin/parser/api"
	"github.com/docker/swarm/scheduler"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type notFoundError struct {
	key  string
	elem string
}

func (nf notFoundError) Error() string {
	if nf.key != "" {
		return fmt.Sprintf("%s not found %s", nf.elem, nf.key)
	}

	return fmt.Sprintf("%s not found", nf.elem)
}

func newNotFound(elem, key string) notFoundError {
	return notFoundError{
		elem: elem,
		key:  key,
	}
}

// Garden is exported.
type Garden struct {
	*sync.Mutex
	ormer        database.Ormer
	kvClient     kvstore.Client
	pluginClient pluginapi.PluginAPI

	cluster.Cluster
	scheduler  *scheduler.Scheduler
	tlsConfig  *tls.Config
	authConfig *types.AuthConfig
}

// NewGarden is exported.
func NewGarden(kvc kvstore.Client, cl cluster.Cluster,
	scheduler *scheduler.Scheduler, ormer database.Ormer,
	pClient pluginapi.PluginAPI, tlsConfig *tls.Config) *Garden {
	return &Garden{
		Mutex:        new(sync.Mutex),
		kvClient:     kvc,
		Cluster:      cl,
		ormer:        ormer,
		pluginClient: pClient,
		scheduler:    scheduler,
		tlsConfig:    tlsConfig,
	}
}

// KVClient returns kv store Client
func (gd *Garden) KVClient() kvstore.Client {
	return gd.kvClient
}

// AuthConfig returns *types.AuthConfig query from db.
func (gd *Garden) AuthConfig() (*types.AuthConfig, error) {
	return gd.ormer.GetAuthConfig()
}

// Ormer returns db orm client.
func (gd *Garden) Ormer() database.Ormer {
	return gd.ormer
}

// PluginClient returns plugin HTTP client.
func (gd *Garden) PluginClient() pluginapi.PluginAPI {
	return gd.pluginClient
}

// TLSConfig returns *tls.Config
func (gd *Garden) TLSConfig() *tls.Config {
	return gd.tlsConfig
}

// NewService is exported.
func (gd *Garden) NewService(spec *structs.ServiceSpec, svc *database.Service) *Service {
	if spec == nil && svc == nil {
		return nil
	}

	return newService(spec, svc, gd.ormer, gd.Cluster, gd.pluginClient)
}

// Service get ServiceInfo from db,convert to ServiceSpec
func (gd *Garden) Service(nameOrID string) (*Service, error) {
	info, err := gd.ormer.GetServiceInfo(nameOrID)
	if err != nil {
		return nil, err
	}

	spec := ConvertServiceInfo(info, gd.Cluster.Containers())

	svc := gd.NewService(&spec, &info.Service)

	return svc, nil
}

// ServiceSpec get ServiceInfo from db,convert to ServiceSpec
func (gd *Garden) ServiceSpec(nameOrID string) (structs.ServiceSpec, error) {
	info, err := gd.ormer.GetServiceInfo(nameOrID)
	if err != nil {
		return structs.ServiceSpec{}, err
	}

	spec := ConvertServiceInfo(info, gd.Cluster.Containers())

	return spec, nil
}

// ListServices returns all ServiceSpec query from db.
func (gd *Garden) ListServices(ctx context.Context) ([]structs.ServiceSpec, error) {
	list, err := gd.ormer.ListServicesInfo()
	if err != nil {
		return nil, err
	}

	containers := gd.Cluster.Containers()

	out := make([]structs.ServiceSpec, 0, len(list))

	for i := range list {
		spec := ConvertServiceInfo(list[i], containers)
		out = append(out, spec)
	}

	return out, nil
}

// DeployService deploy service
func (gd *Garden) DeployService(ctx context.Context,
	svc *Service, compose bool,
	task *database.Task, auth *types.AuthConfig) error {

	deploy := func() error {
		actor := alloc.NewAllocator(gd.ormer, gd.Cluster)
		pendings, err := gd.allocation(ctx, actor, svc, nil, true, true)
		if err != nil {
			return err
		}

		err = svc.createContainer(ctx, pendings, auth)
		if err != nil {
			return err
		}

		err = svc.initStart(ctx, nil, gd.kvClient, nil, nil)
		if err != nil {
			return err
		}

		if compose {
			err = svc.Compose(ctx)
		}

		return err
	}

	sl := tasklock.NewServiceTask(database.ServiceDeployTask, svc.ID(), svc.so, task,
		statusServiceAllocating, statusInitServiceStarted, statusInitServiceStartFailed)

	sl.Before = func(key string, new int, t *database.Task, f func(val int) bool) (bool, int, error) {
		return svc.so.ServiceStatusCAS(key, new, nil, f)
	}

	return sl.Run(
		func(val int) bool {
			return val == statusServcieBuilding
		},
		func() error {
			return deploy()
		},
		false)
}

// Register set Garden,returns a error if has registered in database
func (gd *Garden) Register(req structs.RegisterDC) error {
	sys, err := gd.ormer.GetSysConfig()
	if err == nil {
		return errors.Errorf("DC has registered,dc=%d", sys.ID)
	}

	config := database.SysConfig{
		ID:        req.ID,
		BackupDir: req.BackupDir,
		Retry:     req.Retry,
		Ports: database.Ports{
			Docker:     req.DockerPort,
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

	err = pingHorus(nil, kvc)
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

func pingHorus(ctx context.Context, kvc kvstore.Client) error {
	addr, err := kvc.GetHorusAddr(ctx)
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
