package gardener

import (
	"sync/atomic"

	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/gardener/database"
	consulapi "github.com/hashicorp/consul/api"
)

const pluginPort = 10000

type Configurer interface {
	Parse(buf []byte) error
	ParseFile(file string) error
	Merge(buf []byte) error
	Verify() error
	Set(buf []byte) error
	Marshal() ([]byte, error)
}

var _ Configurer = &mysqlConfig{}

type mysqlConfig struct{}

func (*mysqlConfig) Parse(buf []byte) error      { return nil }
func (*mysqlConfig) ParseFile(file string) error { return nil }
func (*mysqlConfig) Merge(buf []byte) error      { return nil }
func (*mysqlConfig) Verify() error               { return nil }
func (*mysqlConfig) Set(buf []byte) error        { return nil }
func (*mysqlConfig) Marshal() ([]byte, error)    { return nil, nil }

type Operator interface {
	HealthCheck() error
	CopyConfig() error
	StartService() error
	StopService() error
	Recover(file string) error
	Backup() error
	CreateUsers() error
	Migrate(e *cluster.Engine, config *cluster.ContainerConfig) (string, error)
}

type mysqlOperation struct {
	unit      *database.Unit
	eng       *cluster.Engine
	config    *cluster.ContainerConfig
	consulCfg *consulapi.Config
}

type unit struct {
	database.Unit
	Ports      map[string]int
	eng        *cluster.Engine
	config     *cluster.ContainerConfig
	configures map[string]interface{}
	cmd        map[string]string

	Configurer
	Operator
}

func (u *unit) prepareCreateContainer(retry int) error {
	if retry <= 0 {
		retry = 1
	}

	for retry >= 0 {
		// create container
		if !atomic.CompareAndSwapUint32(&u.Status, 0, 1) {
			break
		}

		//if err := u.createNetworkings(retry); err != nil {
		//	break
		//}

		// create volumes
		// if err := u.createVolumes(retry); err != nil {
		// 	break
		// }

		atomic.AddUint32(&u.Status, 1)
		break
	}

	return nil
}

func newVolumeCreateRequest(name, driver string, opts map[string]string) types.VolumeCreateRequest {
	if opts == nil {
		opts = make(map[string]string)
	}
	return types.VolumeCreateRequest{
		Name:       name,
		Driver:     driver,
		DriverOpts: opts,
	}
}
