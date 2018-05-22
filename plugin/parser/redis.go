package parser

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

const redisConfigLine = 100

func init() {
	register("redis", "2.0", &redisConfigV200{})
	register("redis", "3.2", &redisConfig{})

	register("upredis", "1.0", &upredisConfig{})
	register("upredis", "1.1", &upredisConfig{})
	register("upredis", "1.2", &upredisConfig{})
	register("upredis", "1.3", &upredisConfig{})

	register("upredis", "2.0", &upredisConfig{})
	register("upredis", "2.1", &upredisConfig{})
	register("upredis", "2.2", &upredisConfig{})
	register("upredis", "2.3", &upredisConfig{})
}

type redisConfig struct {
	template *structs.ConfigTemplate
	config   map[string]string
}

func (redisConfig) clone(t *structs.ConfigTemplate) parser {
	return &redisConfig{
		template: t,
		config:   make(map[string]string, redisConfigLine),
	}
}

func (c redisConfig) get(key string) (string, bool) {
	if c.config == nil {
		return "", false
	}

	if val, ok := c.config[key]; ok {
		return val, true
	}

	if c.template != nil {
		for i := range c.template.Keysets {
			if c.template.Keysets[i].Key == key {
				return c.template.Keysets[i].Default, false
			}
		}
	}

	return "", false
}

func (c *redisConfig) set(key string, val interface{}) error {
	if c.config == nil {
		c.config = make(map[string]string, redisConfigLine)
	}

	c.config[strings.ToLower(key)] = fmt.Sprintf("%v", val)

	return nil
}

func (c redisConfig) Validate(data map[string]interface{}) error {
	return nil
}

func parseRedisConfig(data []byte) (map[string]string, error) {
	config := make(map[string]string, redisConfigLine)

	lines := bytes.Split(data, []byte{'\n'})
	for _, l := range lines {
		if bytes.HasPrefix(l, []byte{'#'}) {
			continue
		}

		line := bytes.TrimSpace(l)
		if len(line) == 0 {
			continue
		}

		parts := bytes.SplitN(line, []byte{' '}, 2)
		if len(parts) == 2 {
			config[string(parts[0])] = string(bytes.TrimSpace(parts[1]))
		}
	}

	return config, nil
}

func (c *redisConfig) ParseData(data []byte) error {
	cnf, err := parseRedisConfig(data)
	if err != nil {
		return err
	}

	c.config = cnf

	return nil
}

func (c *redisConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	if len(spec.Networking) >= 1 {
		c.config["bind"] = spec.Networking[0].IP
	} else {
		return errors.New("miss ip")
	}

	{
		val, ok := desc.Options["port"]
		if !ok {
			return errors.New("miss port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss port")
		}

		c.config["port"] = strconv.Itoa(port)
	}

	c.config["maxmemory"] = strconv.Itoa(int(float64(spec.Config.HostConfig.Memory) * 0.75))

	if c.template != nil {
		c.config["dir"] = c.template.DataMount
		c.config["pidfile"] = filepath.Join(c.template.DataMount, "redis.pid")
		c.config["logfile"] = filepath.Join(c.template.DataMount, "redis.log")
	}

	return nil
}

func (redisConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/redis-init.sh"}

	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	return cmds, nil
}

func (c redisConfig) Marshal() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	for key, val := range c.config {
		_, err := buffer.WriteString(key + " " + val + "\n")
		if err != nil {
			return buffer.Bytes(), err
		}
	}

	return buffer.Bytes(), nil
}

func (c redisConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return structs.ServiceRegistration{}, err
	}

	reg := structs.HorusRegistration{}
	reg.Service.Select = true
	reg.Service.Name = spec.ID
	reg.Service.Type = "unit_" + desc.Image.Name
	reg.Service.Tag = desc.ID
	reg.Service.Container.Name = spec.Name
	reg.Service.Container.HostName = spec.Engine.Node

	return structs.ServiceRegistration{Horus: &reg}, nil
}

type redisConfigV200 struct {
	redisConfig
}

func (redisConfigV200) clone(t *structs.ConfigTemplate) parser {
	pr := &redisConfigV200{}
	pr.template = t
	pr.config = make(map[string]string, redisConfigLine)

	return pr
}

func (c *redisConfigV200) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	if len(spec.Networking) >= 1 {
		c.config["bind"] = spec.Networking[0].IP
	} else {
		return errors.New("miss ip")
	}

	{
		val, ok := desc.Options["port"]
		if !ok {
			return errors.New("miss port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss port")
		}

		c.config["port"] = strconv.Itoa(port)
	}

	c.config["maxmemory"] = strconv.Itoa(int(float64(spec.Config.HostConfig.Memory) * 0.75))

	if c.template != nil {
		c.config["dir"] = c.template.DataMount
		c.config["pidfile"] = filepath.Join(c.template.DataMount, "redis.pid")
		c.config["logfile"] = filepath.Join(c.template.LogMount, "redis.log")
		c.config["unixsocket"] = filepath.Join(c.template.DataMount, "redis.sock")
	}

	c.config["dbfilename"] = spec.Name + "-dump.rdb"
	c.config["appendfilename"] = spec.Name + "-appendonly.aof"

	return nil
}

type upredisConfig struct {
	redisConfig
}

func (upredisConfig) clone(t *structs.ConfigTemplate) parser {
	pr := &upredisConfig{}
	pr.template = t
	pr.config = make(map[string]string, redisConfigLine)

	return pr
}

func (c *upredisConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	if len(spec.Networking) >= 1 {
		c.config["bind"] = spec.Networking[0].IP
	} else {
		return errors.New("miss ip")
	}

	{
		val, ok := desc.Options["port"]
		if !ok {
			return errors.New("miss port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss port")
		}

		c.config["port"] = strconv.Itoa(port)
	}

	c.config["maxmemory"] = strconv.Itoa(int(float64(spec.Config.HostConfig.Memory) * 0.75))

	if c.template != nil {
		c.config["dir"] = c.template.DataMount
		c.config["pidfile"] = filepath.Join(c.template.DataMount, "upredis.pid")
		c.config["logfile"] = filepath.Join(c.template.LogMount, "upredis.log")
		c.config["unixsocket"] = filepath.Join(c.template.DataMount, "upredis.sock")
	}

	//	c.config["requirepass"] = ""
	//	c.config["masterauth"] = ""
	c.config["dbfilename"] = spec.Name + "-dump.rdb"
	c.config["appendfilename"] = spec.Name + "-appendonly.aof"

	return nil
}

func (upredisConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/upredis-init.sh"}

	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	return cmds, nil
}
