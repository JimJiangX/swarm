package parser

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

func init() {
	register("proxy", "1.0", &proxyConfig{})
	register("proxy", "1.0.2", &proxyConfigV102{})
	register("proxy", "1.1.0", &proxyConfigV110{})
	register("proxy", "1.2.6", &proxyConfigV110{})

	register("upproxy", "1.0", &upproxyConfigV100{})
	register("upproxy", "1.1", &upproxyConfigV100{})
	register("upproxy", "1.2", &upproxyConfigV100{})
	register("upproxy", "1.3", &upproxyConfigV100{})
	register("upproxy", "1.4", &upproxyConfigV100{})
	register("upproxy", "1.5", &upproxyConfigV100{})

	register("upproxy", "2.0", &upproxyConfigV200{})
	register("upproxy", "2.1", &upproxyConfigV200{})
	register("upproxy", "2.2", &upproxyConfigV200{})
	register("upproxy", "2.3", &upproxyConfigV200{})
	register("upproxy", "2.4", &upproxyConfigV200{})
	register("upproxy", "2.5", &upproxyConfigV200{})
}

type proxyConfig struct {
	template *structs.ConfigTemplate
	config   config.Configer
}

func (proxyConfig) clone(t *structs.ConfigTemplate) parser {
	return &proxyConfig{template: t}
}

func (proxyConfig) Validate(data map[string]interface{}) error { return nil }

func (c proxyConfig) get(key string) string {
	if c.config == nil {
		return ""
	}

	if val := c.config.String(key); val != "" {
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

func (c *proxyConfig) set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("proxyConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c *proxyConfig) ParseData(data []byte) error {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return errors.Wrap(err, "parse ini")
	}

	c.config = configer

	return nil
}

func (c *proxyConfig) Marshal() ([]byte, error) {
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, errors.Wrap(err, "create tempFile")
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	err = c.config.SaveConfigFile(tmpfile.Name())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	data, err := ioutil.ReadFile(tmpfile.Name())
	if err == nil {
		return data, nil
	}

	return data, errors.Wrap(err, "read file")
}

func (c proxyConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
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

	var mon *structs.User

	if len(desc.Users) > 0 {
		for i := range desc.Users {
			if desc.Users[i].Role == "mon" {
				mon = &desc.Users[i]
				break
			}
		}

		if mon != nil {
			reg.Service.MonitorUser = mon.Name
			reg.Service.MonitorPassword = mon.Password
		}
	}

	return structs.ServiceRegistration{Horus: &reg}, nil
}

func (c *proxyConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	m["upsql-proxy::proxy-domain"] = desc.ID
	m["upsql-proxy::proxy-name"] = spec.Name

	addr := "localhost"
	if len(spec.Networking) > 0 {
		addr = spec.Networking[0].IP
	}

	{
		val, ok := desc.Options["proxy_data_port"]
		if !ok {
			return errors.New("miss proxy_data_port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss proxy_data_port")
		}

		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", addr, port)
	}

	adminPort := 0
	{
		val, ok := desc.Options["proxy_admin_port"]
		if !ok {
			return errors.New("miss proxy_admin_port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss proxy_admin_port")
		}

		adminPort = port
	}

	m["adm-cli::proxy_admin_port"] = adminPort
	m["adm-cli::adm-cli-address"] = fmt.Sprintf("%s:%v", addr, adminPort)

	m["upsql-proxy::event-threads-count"] = 1
	if spec.Config != nil {
		ncpu, err := spec.Config.CountCPU()
		if err == nil {
			m["upsql-proxy::event-threads-count"] = ncpu
		}
	}

	if addr, ok := desc.Options["adm-cli::adm-svr-address"]; ok {
		m["adm-cli::adm-svr-address"] = addr
	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

func (c proxyConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"/bin/bash"}
	cmds[structs.InitServiceCmd] = []string{"/root/proxy-init.sh"}
	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}
	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	return cmds, nil
}

type proxyConfigV102 struct {
	proxyConfig
}

func (proxyConfigV102) clone(t *structs.ConfigTemplate) parser {
	pr := &proxyConfigV102{}
	pr.template = t

	return pr
}

type proxyConfigV110 struct {
	proxyConfig
}

func (proxyConfigV110) clone(t *structs.ConfigTemplate) parser {
	pr := &proxyConfigV110{}
	pr.template = t

	return pr
}

func (c *proxyConfigV110) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	m["upsql-proxy::proxy-domain"] = desc.Tag
	m["upsql-proxy::proxy-name"] = spec.Name

	addr := "localhost"
	if len(spec.Networking) > 0 {
		addr = spec.Networking[0].IP
	}

	{
		val, ok := desc.Options["proxy_data_port"]
		if !ok {
			return errors.New("miss proxy_data_port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss proxy_data_port")
		}

		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", addr, port)
	}

	val, ok := desc.Options["proxy_admin_port"]
	if ok {
		adminPort, err := atoi(val)
		if err != nil || adminPort == 0 {
			return errors.Wrap(err, "miss proxy_admin_port")
		}

		m["adm-cli::proxy_admin_port"] = adminPort
		m["adm-cli::adm-cli-address"] = fmt.Sprintf("%s:%v", addr, adminPort)
		m["supervise::supervise-address"] = fmt.Sprintf("%s:%v", addr, adminPort)
	}

	m["upsql-proxy::event-threads-count"] = 1
	if spec.Config != nil {
		ncpu, err := spec.Config.CountCPU()
		if err == nil {
			m["upsql-proxy::event-threads-count"] = ncpu
		}
	}

	if addr, ok := desc.Options["adm-cli::adm-svr-address"]; ok {
		m["adm-cli::adm-svr-address"] = addr
	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

type upproxyConfigV100 struct {
	template *structs.ConfigTemplate
	config   config.Configer
}

func (upproxyConfigV100) clone(t *structs.ConfigTemplate) parser {
	return &upproxyConfigV100{template: t}
}

func (upproxyConfigV100) Validate(data map[string]interface{}) error { return nil }

func (c upproxyConfigV100) get(key string) string {
	if c.config == nil {
		return ""
	}

	if val := c.config.String(key); val != "" {
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

func (c *upproxyConfigV100) set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("upproxyConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c *upproxyConfigV100) ParseData(data []byte) error {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return errors.Wrap(err, "parse ini")
	}

	c.config = configer

	return nil
}

func (c *upproxyConfigV100) Marshal() ([]byte, error) {
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, errors.Wrap(err, "create tempFile")
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	err = c.config.SaveConfigFile(tmpfile.Name())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	data, err := ioutil.ReadFile(tmpfile.Name())
	if err == nil {
		return data, nil
	}

	return data, errors.Wrap(err, "read file")
}

func (c upproxyConfigV100) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
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

	var mon *structs.User

	if len(desc.Users) > 0 {
		for i := range desc.Users {
			if desc.Users[i].Role == "mon" {
				mon = &desc.Users[i]
				break
			}
		}

		if mon != nil {
			reg.Service.MonitorUser = mon.Name
			reg.Service.MonitorPassword = mon.Password
		}
	}

	return structs.ServiceRegistration{Horus: &reg}, nil
}

func (c *upproxyConfigV100) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	m["upsql-proxy::proxy-domain"] = desc.Tag
	m["upsql-proxy::proxy-name"] = spec.Name

	addr := "127.0.0.1"
	if len(spec.Networking) > 0 {
		addr = spec.Networking[0].IP
	}

	{
		val, ok := desc.Options["upsql-proxy::proxy_data_port"]
		if !ok {
			return errors.New("miss upsql-proxy::proxy_data_port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss upsql-proxy::proxy_data_port")
		}

		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", addr, port)
	}

	m["upsql-proxy::event-threads-count"] = 1
	if spec.Config != nil {
		ncpu, err := spec.Config.CountCPU()
		if err == nil {
			m["upsql-proxy::event-threads-count"] = ncpu
		}
	}

	if c.template != nil {
		m["upsql-proxy::topology-config"] = filepath.Join(c.template.DataMount, "/topology.json")

		m["log::log-dir"] = c.template.LogMount
	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

func (c upproxyConfigV100) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"/bin/bash"}
	cmds[structs.InitServiceCmd] = []string{"/root/upproxy-init.sh"}
	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}
	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	return cmds, nil
}

type upproxyConfigV200 struct {
	upproxyConfigV100
}

func (upproxyConfigV200) clone(t *structs.ConfigTemplate) parser {
	pr := &upproxyConfigV200{}
	pr.template = t

	return pr
}

func (c *upproxyConfigV200) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	var (
		seq, exist = 0, false
		spec       structs.UnitSpec
	)

	for seq = range desc.Units {
		if id == desc.Units[seq].ID {
			spec = desc.Units[seq]
			exist = true
			break
		}
	}
	if !exist {
		return errors.Errorf("not found unit '%s'", id)
	}

	m := make(map[string]interface{}, 10)

	m["upsql-proxy::proxy-domain"] = desc.Tag
	m["upsql-proxy::proxy-name"] = fmt.Sprintf("%s-%d", desc.Tag, seq)
	m["upsql-proxy::proxy-id"] = strconv.Itoa(seq)

	addr := "127.0.0.1"
	if len(spec.Networking) > 0 {
		addr = spec.Networking[0].IP
	}

	{
		val, ok := desc.Options["upsql-proxy::proxy_data_port"]
		if !ok {
			return errors.New("miss upsql-proxy::proxy_data_port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss upsql-proxy::proxy_data_port")
		}

		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", addr, port)
	}

	m["upsql-proxy::event-threads-count"] = 1
	if spec.Config != nil {
		ncpu, err := spec.Config.CountCPU()
		if err == nil {
			m["upsql-proxy::event-threads-count"] = ncpu
		}
	}

	if c.template != nil {
		m["upsql-proxy::topology-config"] = filepath.Join(c.template.DataMount, "/topology.json")

		m["log::log-dir"] = c.template.LogMount
	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}
