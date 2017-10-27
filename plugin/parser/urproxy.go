package parser

import (
	"strings"

	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

func init() {
	register("upredis-proxy", "1.0", &upredisProxyConfig{})
	register("upredis-proxy", "1.1", &upredisProxyConfig{})
	register("upredis-proxy", "1.2", &upredisProxyConfig{})
	register("upredis-proxy", "1.3", &upredisProxyConfig{})
}

/*
# cat urproxy/redis-proxy.conf.temp.1.1.0.0
<ID>:
  auto_eject_hosts: false
  distribution: modula
  hash: fnv1a_64
  listen: <IP>:<PORT>
  preconnect: true
  redis: true
  redis_auth: <PWD>
  timeout: 400
  server_connections: 1
  #servers:
  #- 146.33.20.23:64000:1 master1
  sentinels:
  - <S1>:<S_PORT>
  - <S2>:<S_PORT>
  - <S3>:<S_PORT>
  white_list:
  -
  black_list:
  -
*/

type upredisProxy struct {
	AutoEjectHosts    bool     `yaml:"auto_eject_hosts"`
	Distribution      string   `yaml:"distribution"`
	Hash              string   `yaml:"hash"`
	Listen            string   `yaml:"listen"`
	Preconnect        bool     `yaml:"preconnect"`
	Redis             bool     `yaml:"redis"`
	RedisAuth         string   `yaml:"redis_auth"`
	Timeout           int      `yaml:"timeout"`
	ServerConnections int      `yaml:"server_connections"`
	Servers           []string `yaml:"servers"`
	Sentinels         []string `yaml:"sentinels"`
	WhiteList         []string `yaml:"white_list"`
	BlackList         []string `yaml:"black_list"`
}

type upredisProxyConfig struct {
	template     *structs.ConfigTemplate
	upredisProxy map[string]upredisProxy
}

func (upredisProxyConfig) clone(t *structs.ConfigTemplate) parser {
	return &sentinelConfig{
		template: t,
	}
}

func (c upredisProxyConfig) get(key string) string {

	switch strings.ToLower(key) {
	case "auto_eject_hosts":
	case "distribution":
	case "hash":
	case "listen":
	case "preconnect":
	case "redis":
	case "redis_auth":
	case "timeout":
	case "server_connections":
	case "servers":
	case "sentinels":
	case "white_list":
	case "black_list":
	}

	return ""
}

func (c *upredisProxyConfig) set(key string, val interface{}) error {
	if c.upredisProxy == nil {
		c.upredisProxy = make(map[string]upredisProxy)
	}

	header := ""
	parts := strings.SplitN(key, "::", 2)
	if len(parts) == 2 {
		header = parts[0]
		key = parts[1]
	}

	if header != "" {
		_, ok := c.upredisProxy[header]
		if !ok && len(c.upredisProxy) > 0 {
			return errors.Errorf("undefined ID:%s", header)
		}
	} else if len(c.upredisProxy) > 0 {
		for header = range c.upredisProxy {
			break
		}
	} else {
		return errors.New("key without ID")
	}

	switch strings.ToLower(key) {
	case "auto_eject_hosts":
	case "distribution":
	case "hash":
	case "listen":
	case "preconnect":
	case "redis":
	case "redis_auth":
	case "timeout":
	case "server_connections":
	case "servers":
	case "sentinels":
	case "white_list":
	case "black_list":
	}

	return nil
}

func (upredisProxyConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *upredisProxyConfig) ParseData(data []byte) error {
	config := map[string]upredisProxy{}

	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return errors.Wrap(err, "parse upredis proxy config")
	}

	c.upredisProxy = config

	return nil
}

func (c *upredisProxyConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {

	return nil
}

func (upredisProxyConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	return cmds, nil
}

func (c upredisProxyConfig) Marshal() ([]byte, error) {
	if c.upredisProxy == nil {
		return nil, nil
	}

	return yaml.Marshal(c.upredisProxy)
}

func (c upredisProxyConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
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
