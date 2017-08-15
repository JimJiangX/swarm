package parser

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

func init() {
	register("switch_manager", "1.0", &switchManagerConfig{})
	register("switch_manager", "1.1.19", &switchManagerConfigV1119{})
	register("switch_manager", "1.1.23", &switchManagerConfigV1123{})
}

type switchManagerConfig struct {
	template *structs.ConfigTemplate
	config   config.Configer
}

func (switchManagerConfig) clone(t *structs.ConfigTemplate) parser {
	return &switchManagerConfig{template: t}
}

func (switchManagerConfig) Validate(data map[string]interface{}) error { return nil }

func (c *switchManagerConfig) ParseData(data []byte) error {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return errors.Wrap(err, "parse ini")
	}

	c.config = configer

	return nil
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
	//	if c.config == nil || len(args) == 0 {
	//		return healthCheck{}, errors.New("params not ready")
	//	}

	//	port, err := c.config.Int("Port")
	//	if err != nil {
	//		return healthCheck{}, errors.Wrap(err, "get 'Port'")
	//	}

	//	return healthCheck{
	//		Addr:     "",
	//		Port:     port,
	//		Script:   "/opt/DBaaS/script/check_switchmanager.sh " + args[0],
	//		Shell:    "",
	//		Interval: "10s",
	//		TTL:      "",
	//		Tags:     nil,
	//	}, nil

	return structs.ServiceRegistration{}, nil
}

func (c switchManagerConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	//	m["domain"] = svc.ID
	//	m["name"] = u.Name
	//	port, proxyPort := 0, 0
	//	for i := range u.ports {
	//		if u.ports[i].Name == "Port" {
	//			port = u.ports[i].Port
	//		} else if u.ports[i].Name == "ProxyPort" {
	//			proxyPort = u.ports[i].Port
	//		}
	//	}
	//	m["ProxyPort"] = proxyPort
	//	m["Port"] = port

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

	cmds[structs.InitServiceCmd] = []string{"/root/swm.service", "start"}

	cmds[structs.StartServiceCmd] = []string{"/root/swm.service", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/swm.service", "stop"}

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

func (c switchManagerConfigV1123) GenerateConfig(id string, desc structs.ServiceSpec) error {

	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	for key, val := range desc.Options {
		_ = key
		_ = val
	}
	//	m["domain"] = svc.ID
	//	m["name"] = u.Name
	//	port, proxyPort := 0, 0
	//	for i := range u.ports {
	//		if u.ports[i].Name == "Port" {
	//			port = u.ports[i].Port
	//		} else if u.ports[i].Name == "ProxyPort" {
	//			proxyPort = u.ports[i].Port
	//		}
	//	}
	//	m["ProxyPort"] = proxyPort
	//	m["Port"] = port

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
