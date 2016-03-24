package swarm

import (
	"golang.org/x/net/context"

	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/samalba/dockerclient"
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
	Migrate(e *cluster.Engine, config *cluster.ContainerConfig) (*cluster.Container, error)
}

type mysqlOperation struct {
	unit      *database.Unit
	engine    *cluster.Engine
	config    *cluster.ContainerConfig
	consulCfg *consulapi.Config
}

func (mysql *mysqlOperation) HealthCheck() error {
	return nil
}
func (mysql *mysqlOperation) CopyConfig() error {
	return nil
}
func (mysql *mysqlOperation) StartService() error {
	return nil
}
func (mysql *mysqlOperation) StopService() error {
	return nil
}
func (mysql *mysqlOperation) Recover(file string) error {
	return nil
}
func (mysql *mysqlOperation) Backup() error {
	return nil
}

func (mysql *mysqlOperation) Migrate(e *cluster.Engine, config *cluster.ContainerConfig) (*cluster.Container, error) {
	return nil, nil
}

type unit struct {
	retry int64
	database.Unit
	engine     *cluster.Engine
	config     *cluster.ContainerConfig
	container  *cluster.Container
	authConfig *dockerclient.AuthConfig
	ports      []database.Port
	configures map[string]interface{}
	cmd        map[string]string

	Configurer
	Operator
}

func (u *unit) prepareCreateContainer() error {

	return nil
}

func (u *unit) createContainer(authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {

	container, err := u.engine.Create(u.config, u.Unit.Name, true, u.authConfig)
	if err == nil && container != nil {
		u.container = container
		u.Unit.ContainerID = container.Id
	}

	return container, err
}

func (u *unit) updateContainer(updateConfig container.UpdateConfig) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerUpdate(context.TODO(), u.container.Id, updateConfig)
}

func (u *unit) removeContainer(force, rmVolumes bool) error {
	err := u.engine.RemoveContainer(u.container, force, rmVolumes)
	if err != nil {
		err = u.engine.RemoveContainer(u.container, true, rmVolumes)
	}

	return err
}

func (u *unit) startContainer() error {
	return u.engine.StartContainer(u.Unit.ContainerID, nil)
}

func (u *unit) stopContainer(timeout int) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerStop(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) restartContainer(timeout int) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerRestart(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) RenameContainer(name string) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerRename(context.TODO(), u.container.Id, u.Unit.Name)
}

func (u *unit) exec(cmd []string) error {
	client := u.engine.EngineAPIClient()

	resp, err := client.ContainerExecCreate(context.TODO(), types.ExecConfig{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          cmd,
		Container:    u.container.Id,
		Detach:       false,
	})

	if err != nil {
		return err
	}

	return client.ContainerExecStart(context.TODO(), resp.ID, types.ExecStartCheck{})
}

func (u *unit) createNetworking() error {
	return nil
}

func (u *unit) removeNetworking() error {
	return nil
}

func (u *unit) createVolume() (*cluster.Volume, error) {
	return nil, nil
}

func (u *unit) updateVolume() error {
	return nil
}

func (u *unit) removeVolume() error {
	return nil
}

func (u *unit) createVG() error {
	return nil
}

func (u *unit) activateVG() error {
	return nil
}

func (u *unit) deactivateVG() error {
	return nil
}

func (u *unit) extendVG() error {
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
