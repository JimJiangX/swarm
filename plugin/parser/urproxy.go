package parser

import (
	"fmt"
	"strconv"
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
const stringAndString = "&&&"

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
	return &upredisProxyConfig{
		template: t,
	}
}

func (c upredisProxyConfig) get(key string) string {
	if c.upredisProxy == nil {
		return ""
	}

	header, key, err := c.header(key)
	if err != nil {
		return ""
	}

	obj := c.upredisProxy[header]

	switch strings.ToLower(key) {
	case "auto_eject_hosts":
		if obj.AutoEjectHosts {
			return "true"
		}

		return "false"

	case "distribution":
		return obj.Distribution

	case "hash":
		return obj.Hash

	case "listen":
		return obj.Listen

	case "preconnect":
		if obj.Preconnect {
			return "true"
		}

		return "false"

	case "redis":
		if obj.Redis {
			return "true"
		}

		return "false"

	case "redis_auth":
		return obj.RedisAuth

	case "timeout":
		return fmt.Sprintf("%v", obj.Timeout)

	case "server_connections":
		return fmt.Sprintf("%v", obj.ServerConnections)
	case "servers":
		return strings.Join(obj.Servers, stringAndString)

	case "sentinels":
		return strings.Join(obj.Sentinels, stringAndString)

	case "white_list":
		return strings.Join(obj.WhiteList, stringAndString)

	case "black_list":
		return strings.Join(obj.BlackList, stringAndString)
	}

	return ""
}

func (c upredisProxyConfig) header(key string) (string, string, error) {
	header := "default"

	parts := strings.SplitN(key, "::", 2)
	if len(parts) == 2 {
		header = parts[0]
		key = parts[1]
	}

	if header != "" {
		_, ok := c.upredisProxy[header]
		if !ok && len(c.upredisProxy) > 0 {
			return "", "", errors.Errorf("undefined ID:%s", header)
		}
	} else if len(c.upredisProxy) > 0 {
		for header = range c.upredisProxy {
			break
		}
	} else {
		return "", "", errors.New("key without HEADER")
	}

	return header, key, nil
}

func boolValue(val interface{}) bool {
	if val == nil {
		return false
	}

	switch val.(type) {
	case bool:
		return val.(bool)
	case int:
		return val.(int) != 0
	case byte:
		return val.(byte) != 0
	case string:
		s := val.(string)
		return !(s == "" || s == "0" || s == "no" || s == "false" || s == "none")
	}

	return false
}

func atoi(v interface{}) (int, error) {
	if v == nil {
		return 0, nil
	}

	n := 0

	switch v.(type) {
	case int:
		n = v.(int)
	case byte:
		n = int(v.(byte))
	case string:
		if s := v.(string); s == "" {
			n = 0
		} else {
			var err error
			n, err = strconv.Atoi(s)
			if err != nil {
				return n, errors.Errorf("parse '%s' => int error,%s", s, err)
			}
		}
	}

	return n, nil
}

func stringSliceValue(in []string, val interface{}) ([]string, error) {
	if val == nil {
		return nil, nil
	}

	switch val.(type) {

	case []string:
		return val.([]string), nil

	case string:
		s := val.(string)
		if s == "" {
			return nil, nil
		}

		if s[0] == '+' {
			in = append(in, s[1:])
		} else if s[0] == '-' {
			s = s[1:]
			out := make([]string, 0, len(in))
			for i := range in {
				if s != in[i] {
					out = append(out, in[i])
				}
			}
			return out, nil
		} else {
			return strings.Split(s, stringAndString), nil
		}
	}

	return in, nil
}

func (c *upredisProxyConfig) set(key string, val interface{}) error {
	if c.upredisProxy == nil {
		c.upredisProxy = make(map[string]upredisProxy)
	}

	header, key, err := c.header(key)
	if err != nil {
		return err
	}

	obj := c.upredisProxy[header]

	switch strings.ToLower(key) {
	case "auto_eject_hosts":
		obj.AutoEjectHosts = boolValue(val)

	case "distribution":
		obj.Distribution = fmt.Sprintf("%v", val)

	case "hash":
		obj.Hash = fmt.Sprintf("%v", val)

	case "listen":
		obj.Hash = fmt.Sprintf("%v", val)

	case "preconnect":
		obj.Preconnect = boolValue(val)

	case "redis":
		obj.Redis = boolValue(val)

	case "redis_auth":
		obj.RedisAuth = fmt.Sprintf("%v", val)

	case "timeout":
		v, err := atoi(val)
		if err != nil {
			return err
		}

		obj.Timeout = v

	case "server_connections":
		v, err := atoi(val)
		if err != nil {
			return err
		}

		obj.ServerConnections = v

	case "servers":
		out, err := stringSliceValue(obj.Servers, val)
		if err != nil {
			return err
		}

		obj.Servers = out

	case "sentinels":
		out, err := stringSliceValue(obj.Sentinels, val)
		if err != nil {
			return err
		}

		obj.Sentinels = out
	case "white_list":
		out, err := stringSliceValue(obj.WhiteList, val)
		if err != nil {
			return err
		}

		obj.WhiteList = out
	case "black_list":
		out, err := stringSliceValue(obj.BlackList, val)
		if err != nil {
			return err
		}

		obj.BlackList = out
	}

	c.upredisProxy[header] = obj

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
	if c.upredisProxy == nil {
		c.upredisProxy = make(map[string]upredisProxy)
	}

	obj := c.upredisProxy[id]

	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	obj.Listen = fmt.Sprintf("%s:%v", spec.Networking[0].IP, desc.Options["port"])

	c.upredisProxy[id] = obj

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
