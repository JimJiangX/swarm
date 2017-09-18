package parser

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/structs"
)

const redisConfigLine = 100

func init() {
	register("redis", "3.2", &redisConfig{})

	register("upredis", "1.0", &upredisConfig{})
	register("upredis", "1.1", &upredisConfig{})
	register("upredis", "1.2", &upredisConfig{})
	register("upredis", "2.0", &upredisConfig{})
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

func (c redisConfig) get(key string) string {
	if c.config == nil {
		return ""
	}

	if val, ok := c.config[key]; ok {
		return val
	}

	if c.template != nil {
		for i := range c.template.Keysets {
			if c.template.Keysets[i].Key == key {
				return c.template.Keysets[i].Default
			}
		}
	}

	return ""
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

func (c *redisConfig) ParseData(data []byte) error {
	if c.config == nil {
		c.config = make(map[string]string, redisConfigLine)
	}

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
			c.config[string(parts[0])] = string(bytes.TrimSpace(parts[1]))
		}
	}

	if c.template != nil {
		c.config["dir"] = c.template.DataMount
		c.config["pidfile"] = filepath.Join(c.template.DataMount, "redis.pid")
		c.config["logfile"] = filepath.Join(c.template.DataMount, "redis.log")
	}

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
	}

	c.config["port"] = fmt.Sprintf("%v", desc.Options["port"])

	c.config["maxmemory"] = strconv.Itoa(int(float64(spec.Config.HostConfig.Memory) * 0.7))

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

	cmds[structs.InitServiceCmd] = []string{"/root/serv", "start"}

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

	im, err := structs.ParseImage(c.template.Image)
	if err != nil {
		return structs.ServiceRegistration{}, err
	}

	reg := structs.HorusRegistration{}
	reg.Service.Select = true
	reg.Service.Name = spec.ID
	reg.Service.Type = "unit_" + im.Name
	reg.Service.Tag = desc.ID
	reg.Service.Container.Name = spec.Name
	reg.Service.Container.HostName = spec.Engine.Node

	return structs.ServiceRegistration{Horus: &reg}, nil
}

type upredisConfig struct {
	redisConfig
}

func (upredisConfig) clone(t *structs.ConfigTemplate) parser {
	pr := &redisConfig{}
	pr.template = t
	pr.config = make(map[string]string, redisConfigLine)

	return pr
}
