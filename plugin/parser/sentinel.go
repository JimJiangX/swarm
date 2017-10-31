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

func init() {
	register("sentinel", "1.0", &sentinelConfig{})
	register("sentinel", "1.1", &sentinelConfig{})
	register("sentinel", "1.2", &sentinelConfig{})
	register("sentinel", "1.3", &sentinelConfig{})
}

type sentinelConfig struct {
	template *structs.ConfigTemplate
	config   map[string]string
}

func (sentinelConfig) clone(t *structs.ConfigTemplate) parser {
	return &sentinelConfig{
		template: t,
		config:   make(map[string]string),
	}
}

func (c sentinelConfig) get(key string) string {
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

func (c *sentinelConfig) set(key string, val interface{}) error {
	if c.config == nil {
		c.config = make(map[string]string)
	}

	c.config[strings.ToLower(key)] = fmt.Sprintf("%v", val)

	return nil
}

func (sentinelConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *sentinelConfig) ParseData(data []byte) error {
	cnf, err := parseRedisConfig(data)
	if err != nil {
		return err
	}

	c.config = cnf

	return nil
}

func (c *sentinelConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	/*
		# cat sentinel/sentinel.conf.temp.1.2.0.0
		daemonize yes
		logfile <LOG_DIR>
		port <PORT>
		dir <DAT_DIR>
		#sentinel monitor mymaster01 <MASTER01_IP> <MASTER01_PORT> <quorum默认填2>
		#sentinel auth-pass mymaster01 <PASS>
		#sentinel down-after-milliseconds mymaster01 5000
		#sentinel parallel-syncs mymaster01 1
		#sentinel failover-timeout mymaster01 25000
	*/
	err := c.Validate(desc.Options)
	if err != nil {
		return err
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

	if c.template != nil {
		c.config["dir"] = c.template.DataMount
		c.config["logfile"] = filepath.Join(c.template.DataMount, "sentinel.log")
	}

	return nil
}

func (sentinelConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	return cmds, nil
}

func (c sentinelConfig) Marshal() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	for key, val := range c.config {
		_, err := buffer.WriteString(key + " " + val + "\n")
		if err != nil {
			return buffer.Bytes(), err
		}
	}

	return buffer.Bytes(), nil
}

func (c sentinelConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
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
