package swarm

import (
	"encoding/json"
	"errors"
	"time"

	"golang.org/x/net/context"

	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/samalba/dockerclient"
)

var _ Configurer = &mysqlConfig{}
var _ Operator = &mysqlOperation{}

const pluginPort = 10000

type Configurer interface {
	Path() string
	Merge(map[string]interface{}) error
	Verify(map[string]interface{}) error
	Set(string, interface{}) error
	Marshal() ([]byte, error)
	SaveToDisk() (string, error)
}

type mysqlConfig struct {
	unit    *database.Unit
	parent  *database.UnitConfig
	content map[string]interface{}
}

func (c *mysqlConfig) Path() string {
	if c.parent == nil {
		return ""
	}

	return c.parent.ConfigFilePath
}

func (c *mysqlConfig) Merge(data map[string]interface{}) error {
	if c.content == nil && c.parent != nil {
		err := json.Unmarshal([]byte(c.parent.Content), &c.content)
		if err != nil {
			// return err
		}
	}

	if c.content == nil {
		c.content = data
		return nil
	}

	for key, val := range data {
		c.content[key] = val
	}

	return nil
}

func (*mysqlConfig) Verify(data map[string]interface{}) error { return nil }
func (c *mysqlConfig) Set(key string, val interface{}) error {
	c.content[key] = val
	return nil
}

func (c *mysqlConfig) Marshal() ([]byte, error) {

	return json.Marshal(c.content)
}

func (c *mysqlConfig) SaveToDisk() (string, error) {
	if err := c.Verify(c.content); err != nil {
		return "", err
	}

	data, err := c.Marshal()
	if err != nil {
		return "", err
	}

	config := database.UnitConfig{
		ID:             generateUUID(64),
		ImageID:        c.unit.ImageID,
		Version:        c.parent.Version + 1,
		ParentID:       c.parent.ID,
		Content:        string(data),
		ConfigFilePath: c.Path(),
		CreateAt:       time.Now(),
	}

	c.unit.ConfigID = config.ID

	err = database.SaveUnitConfigToDisk(c.unit, config)
	if err != nil {
		return "", err
	}

	return config.ID, nil
}

type Operator interface {
	CopyConfig() error
	StartService() error
	StopService() error
	Recover(file string) error
	Backup() error
	Migrate(e *cluster.Engine, config *cluster.ContainerConfig) (*cluster.Container, error)
	RegisterHealthCheck(client *consulapi.Client) error
	DeregisterHealthCheck(client *consulapi.Client) error
}

type mysqlOperation struct {
	unit   *database.Unit
	engine *cluster.Engine
}

func (mysql *mysqlOperation) RegisterHealthCheck(client *consulapi.Client) error {
	return nil
}

func (mysql *mysqlOperation) DeregisterHealthCheck(client *consulapi.Client) error {
	return nil
}

func (mysql *mysqlOperation) CopyConfig() error {
	return nil
}

func (mysql *mysqlOperation) StartService() error {

	cmd := []string{"start mysql service"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
}

func (mysql *mysqlOperation) StopService() error {
	cmd := []string{"stop service"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
}

func (mysql *mysqlOperation) Recover(file string) error {
	cmd := []string{"recover"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
}

func (mysql *mysqlOperation) Backup() error {
	cmd := []string{"backup"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
}

func (mysql *mysqlOperation) Migrate(e *cluster.Engine, config *cluster.ContainerConfig) (*cluster.Container, error) {
	return nil, nil
}

type unit struct {
	retry int64
	database.Unit
	engine       *cluster.Engine
	config       *cluster.ContainerConfig
	container    *cluster.Container
	parentConfig *database.UnitConfig
	ports        []database.Port
	networkings  []IPInfo

	Configurer
	Operator
}

func (u *unit) prepareCreateContainer() error {

	return nil
}

func (u *unit) createContainer(authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
	container, err := u.engine.Create(u.config, u.Unit.Name, true, authConfig)
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
	err := u.StopService()
	if err != nil {
		return err
	}

	client := u.engine.EngineAPIClient()

	return client.ContainerStop(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) restartContainer(timeout int) error {
	err := u.StopService()
	if err != nil {
		return err
	}

	client := u.engine.EngineAPIClient()

	return client.ContainerRestart(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) RenameContainer(name string) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerRename(context.TODO(), u.container.Id, u.Unit.Name)
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

// containerExec
func containerExec(engine *cluster.Engine, containerID string, cmd []string) error {
	client := engine.EngineAPIClient()
	if client == nil {
		return errors.New("Engine APIClient is nil")
	}

	resp, err := client.ContainerExecCreate(context.TODO(), types.ExecConfig{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          cmd,
		Container:    containerID,
		Detach:       false,
	})

	if err != nil {
		return err
	}

	return client.ContainerExecStart(context.TODO(), resp.ID, types.ExecStartCheck{})
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
