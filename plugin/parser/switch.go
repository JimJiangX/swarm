package parser

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

func init() {
	register("switch_manager", "1.0", &switchManagerConfig{})
	register("switch_manager", "1.1.19", &switchManagerConfigV1119{})
	register("switch_manager", "1.1.23", &switchManagerConfigV1123{})
	//	register("switch_manager", "1.1.47", &switchManagerConfigV1147{})
	register("switch_manager", "1.2.0", &switchManagerConfigV120{})
}

var (
	consulPort         = "8500"
	leaderElectionPath = "docker/swarm/leader"
)

func parseKvPath(rawurl string) (string, string) {
	parts := strings.SplitN(rawurl, "://", 2)

	// nodes:port,node2:port => nodes://node1:port,node2:port
	if len(parts) == 1 {
		return "nodes", parts[0]
	}
	return parts[0], parts[1]
}

func setLeaderElectionPath(path string) {
	_, uris := parseKvPath(path)

	parts := strings.SplitN(uris, "/", 2)

	// A custom prefix to the path can be optionally used.
	if len(parts) == 2 {
		leaderElectionPath = parts[1]

		_, port, err := net.SplitHostPort(parts[0])
		if err == nil {
			consulPort = port
		}
	}
}

type switchManagerConfig struct {
	template *structs.ConfigTemplate
	config   config.Configer
}

func (switchManagerConfig) clone(t *structs.ConfigTemplate) parser {
	return &switchManagerConfig{template: t}
}

func (switchManagerConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *switchManagerConfig) ParseData(data []byte) error {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return errors.Wrap(err, "parse ini")
	}

	c.config = configer

	return nil
}

func (c switchManagerConfig) get(key string) string {
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

func (c *switchManagerConfig) set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("switchManagerConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c *switchManagerConfig) Marshal() ([]byte, error) {
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

func (c switchManagerConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
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

func (c *switchManagerConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	m["domain"] = desc.ID
	m["name"] = spec.Name

	m["Port"] = desc.Options["Port"]

	m["ConsulPort"] = consulPort

	// swarm
	m["ConsulPort"] = consulPort
	m["SwarmHostKey"] = leaderElectionPath
	m["SwarmUserAgent"] = "1.31"

	//	// consul
	//	m["ConsulBindNetworkName"] = u.engine.Labels[_Admin_NIC_Lable]
	//	m["ConsulPort"] = sys.ConsulPort

	//	// swarm
	//	m["SwarmUserAgent"] = version.VERSION
	//	m["SwarmHostKey"] = leaderElectionPath

	//	// _User_Check Role
	//	m["swarmhealthcheckuser"] = user.Username
	//	m["swarmhealthcheckpassword"] = user.Password

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

func (c switchManagerConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"/bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/swm-init.sh"}

	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	return cmds, nil
}

type switchManagerConfigV1119 struct {
	switchManagerConfig
}

func (switchManagerConfigV1119) clone(t *structs.ConfigTemplate) parser {
	pr := &switchManagerConfigV1119{}
	pr.template = t

	return pr
}

type switchManagerConfigV1123 struct {
	switchManagerConfig
}

func (switchManagerConfigV1123) clone(t *structs.ConfigTemplate) parser {
	pr := &switchManagerConfigV1123{}
	pr.template = t

	return pr
}

func (c *switchManagerConfigV1123) GenerateConfig(id string, desc structs.ServiceSpec) error {

	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	m["domain"] = desc.ID
	m["name"] = spec.Name

	m["Port"] = desc.Options["Port"]

	// consul
	//	m["ConsulBindNetworkName"] = u.engine.Labels[_Admin_NIC_Lable]
	m["ConsulPort"] = consulPort

	// swarm
	m["ConsulPort"] = consulPort
	m["SwarmHostKey"] = leaderElectionPath
	m["SwarmUserAgent"] = "1.31"

	//	// _User_Check Role
	//	m["swarmhealthcheckuser"] = user.Username
	//	m["swarmhealthcheckpassword"] = user.Password

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

type switchManagerConfigV1147 struct {
	switchManagerConfig
}

func (switchManagerConfigV1147) clone(t *structs.ConfigTemplate) parser {
	pr := &switchManagerConfigV1123{}
	pr.template = t

	return pr
}

func (c *switchManagerConfigV1147) GenerateConfig(id string, desc structs.ServiceSpec) error {

	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	m["domain"] = desc.ID
	m["name"] = spec.Name

	m["Port"] = desc.Options["Port"]
	m["ConsulIP"] = spec.Engine.Addr

	m["ConsulPort"] = consulPort
	m["SwarmHostKey"] = leaderElectionPath
	m["SwarmUserAgent"] = "1.31"

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

type switchManagerConfigV120 struct {
	switchManagerConfig
}

func (switchManagerConfigV120) clone(t *structs.ConfigTemplate) parser {
	pr := &switchManagerConfigV120{}
	pr.template = t

	return pr
}

func (c *switchManagerConfigV120) GenerateConfig(id string, desc structs.ServiceSpec) error {

	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	m["domain"] = desc.ID
	m["name"] = spec.Name

	if port, ok := desc.Options["Port"]; ok {
		m["Port"] = port
	} else {
		return errors.New("miss key:Port")
	}

	m["ConsulIP"] = spec.Engine.Addr

	m["ConsulPort"] = consulPort
	m["SwarmHostKey"] = leaderElectionPath
	m["SwarmUserAgent"] = "1.31"

	if c.template != nil {
		m["SwarmSocketPath"] = filepath.Join(c.template.DataMount, "/upsql.sock")
		m["SwarmHealthCheckConfigFile"] = filepath.Join(c.template.DataMount, "/my.cnf")
	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}
