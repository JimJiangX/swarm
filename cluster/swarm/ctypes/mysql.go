package ctypes

import (
	"encoding/json"
	"errors"
	"time"

	"golang.org/x/net/context"

	"github.com/docker/engine-api/types"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

type mysqlConfig struct {
	unit    *database.Unit
	parent  *database.UnitConfig
	content map[string]interface{}
}

func NewMysqlConfig(unit *database.Unit, parent *database.UnitConfig) *mysqlConfig {
	var content map[string]interface{}
	err := json.Unmarshal([]byte(parent.Content), &content)
	if err != nil {
		// return err
	}

	return &mysqlConfig{
		unit:    unit,
		parent:  parent,
		content: content,
	}
}

func (c *mysqlConfig) Path() string {
	if c.parent == nil {
		return ""
	}

	return c.parent.Path
}

func (c *mysqlConfig) Merge(data map[string]interface{}) error {
	if c.content == nil && c.parent != nil {
		err := json.Unmarshal([]byte(c.parent.Content), &c.content)
		if err != nil {
			return err
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

func (mysqlConfig) verify(data map[string]interface{}) error {
	return nil
}

func (c mysqlConfig) Verify(data map[string]interface{}) error {
	if len(data) > 0 {
		if err := c.verify(data); err != nil {
			return err
		}
	}

	if len(c.content) > 0 {
		if err := c.verify(c.content); err != nil {
			return err
		}
	}

	return nil
}
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
		ID:       utils.Generate64UUID(),
		ImageID:  c.unit.ImageID,
		Version:  c.parent.Version + 1,
		ParentID: c.parent.ID,
		Content:  string(data),
		Path:     c.Path(),
		CreateAt: time.Now(),
	}

	c.unit.ConfigID = config.ID

	err = database.SaveUnitConfigToDisk(c.unit, config)
	if err != nil {
		return "", err
	}

	return config.ID, nil
}

type mysqlOperator struct {
	unit   *database.Unit
	engine *cluster.Engine
}

func NewMysqlOperator(unit *database.Unit, engine *cluster.Engine) *mysqlOperator {
	return &mysqlOperator{
		unit:   unit,
		engine: engine,
	}
}

func (mysql *mysqlOperator) CopyConfig(opt sdk.VolumeFileConfig) error {
	return nil
}

func (mysql *mysqlOperator) StartService() error {

	cmd := []string{"start mysql service"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
}

func (mysql *mysqlOperator) StopService() error {
	cmd := []string{"stop service"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
}

func (mysql *mysqlOperator) Recover(file string) error {
	cmd := []string{"recover"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
}

func (mysql *mysqlOperator) Backup() error {
	cmd := []string{"backup"}

	return containerExec(mysql.engine, mysql.unit.ContainerID, cmd)
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
