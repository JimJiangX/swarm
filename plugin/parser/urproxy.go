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
	register("urproxy", "1.0", &upredisProxyConfig{})
	register("urproxy", "1.1", &upredisProxyConfig{})
	register("urproxy", "1.2", &upredisProxyConfig{})
	register("urproxy", "1.3", &upredisProxyConfig{})

	register("urproxy", "2.0", &upredisProxyConfig{})
	register("urproxy", "2.1", &upredisProxyConfig{})
	register("urproxy", "2.2", &upredisProxyConfig{})
	register("urproxy", "2.3", &upredisProxyConfig{})
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

func (c upredisProxyConfig) get(key string) (string, bool) {
	if c.upredisProxy == nil {
		return "", false
	}

	header, key, err := c.header(key)
	if err != nil {
		return "", false
	}

	obj := c.upredisProxy[header]

	switch strings.ToLower(key) {
	case "auto_eject_hosts":
		if obj.AutoEjectHosts {
			return "true", true
		}

		return "false", true

	case "distribution":
		return obj.Distribution, true

	case "hash":
		return obj.Hash, true

	case "listen":
		return obj.Listen, true

	case "preconnect":
		if obj.Preconnect {
			return "true", true
		}

		return "false", true

	case "redis":
		if obj.Redis {
			return "true", true
		}

		return "false", true

	case "redis_auth":
		return obj.RedisAuth, true

	case "timeout":
		return fmt.Sprintf("%v", obj.Timeout), true

	case "server_connections":
		return fmt.Sprintf("%v", obj.ServerConnections), true

	case "sentinels":
		return strings.Join(obj.Sentinels, stringAndString), true

	case "white_list":
		return strings.Join(obj.WhiteList, stringAndString), true

	case "black_list":
		return strings.Join(obj.BlackList, stringAndString), true
	}

	return "", false
}

func (c upredisProxyConfig) header(key string) (string, string, error) {
	header := "default"

	if key == "" {
		return header, "", nil
	}

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
	}

	return header, key, nil
}

func boolValue(val interface{}) bool {
	if val == nil {
		return false
	}

	switch v := val.(type) {
	case bool:
		return v

	case int8, int32, int64, int, uint8, uint32, uint64, uint:
		return v != 0

	case string:

		return !(v == "" || v == "0" || v == "no" || v == "false" || v == "none")
	}

	return false
}

func atoi(v interface{}) (int, error) {
	if v == nil {
		return 0, nil
	}

	n := 0

	switch val := v.(type) {
	case float32:
		n = int(val)
	case float64:
		n = int(val)
	case int8:
		n = int(val)
	case uint8:
		n = int(val)
	case int32:
		n = int(val)
	case uint32:
		n = int(val)
	case int:
		n = int(val)
	case uint:
		n = int(val)
	case int64:
		n = int(val)
	case uint64:
		n = int(val)

	case string:
		if s := val; s == "" {
			n = 0
		} else {
			var err error
			n, err = strconv.Atoi(s)
			if err != nil {
				return n, errors.Errorf("parse '%s' => int error,%s", s, err)
			}
		}

	default:
		return 0, errors.Errorf("unknown type of input,%T:%v", v, v)
	}

	return n, nil
}

func stringSliceValue(in []string, val interface{}) ([]string, error) {
	if val == nil {
		return nil, nil
	}

	switch v := val.(type) {
	default:
		return nil, errors.Errorf("unknown type of input,%T:%v", v, v)

	case []string:
		return v, nil

	case string:
		if v == "" {
			return nil, nil
		}

		if v[0] == '+' {
			in = append(in, v[1:])
		} else if v[0] == '-' {
			v = v[1:]
			out := make([]string, 0, len(in))
			for i := range in {
				if v != in[i] {
					out = append(out, in[i])
				}
			}
			return out, nil
		} else {
			return strings.Split(v, stringAndString), nil
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
		obj.Listen = fmt.Sprintf("%v", val)

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

	header, _, _ := c.header("")

	if _, ok := config[header]; ok {
		c.upredisProxy = config
	} else {
		for _, cc := range config {
			c.upredisProxy[header] = cc
			break
		}
	}

	return nil
}

func (c *upredisProxyConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	if c.upredisProxy == nil {
		c.upredisProxy = make(map[string]upredisProxy)
	}

	header, _, _ := c.header("")

	obj := c.upredisProxy[header]

	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	val, ok := desc.Options["port"]
	if !ok {
		return errors.New("miss port")
	}
	port, err := atoi(val)
	if err != nil || port == 0 {
		return errors.Wrap(err, "miss port")
	}

	if len(spec.Networking) == 0 {
		return errors.New("miss ip")
	}

	obj.Listen = fmt.Sprintf("%s:%v", spec.Networking[0].IP, port)

	c.upredisProxy[header] = obj

	return nil
}

func (upredisProxyConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/urproxy-init.sh"}

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

	reg := structs.HorusRegistration{}
	reg.Service.Select = true
	reg.Service.Name = spec.ID
	reg.Service.Type = "unit_" + desc.Image.Name
	reg.Service.Tag = desc.ID
	reg.Service.Container.Name = spec.Name
	reg.Service.Container.HostName = spec.Engine.Node

	return structs.ServiceRegistration{Horus: &reg}, nil
}
