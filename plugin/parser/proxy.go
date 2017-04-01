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
	register("proxy", "1.0", &proxyConfig{})
	register("proxy", "1.0.2", &proxyConfigV102{})
	register("proxy", "1.1.0", &proxyConfigV110{})
}

type proxyConfig struct {
	template *structs.ConfigTemplate
	config   config.Configer
}

func (proxyConfig) clone(t *structs.ConfigTemplate) parser {
	return &proxyConfig{template: t}
}

func (proxyConfig) Validate(data map[string]interface{}) error { return nil }

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

//func (proxyConfig) Requirement() structs.RequireResource {
//	ports := []port{
//		port{
//			proto: "tcp",
//			name:  "proxy_data_port",
//		},
//		port{
//			proto: "tcp",
//			name:  "proxy_admin_port",
//		},
//	}
//	nets := []netRequire{
//		netRequire{
//			Type: _ContainersNetworking,
//			num:  1,
//		},
//		netRequire{
//			Type: _ExternalAccessNetworking,
//			num:  1,
//		},
//	}
//	return require{
//		ports:       ports,
//		networkings: nets,
//	}

//	return structs.RequireResource{}
//}

func (c proxyConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
	var spec *structs.UnitSpec

	for i := range desc.Units {
		if id == desc.Units[i].ID {
			spec = &desc.Units[i]
			break
		}
	}

	if spec == nil {
		return structs.ServiceRegistration{}, errors.Errorf("not found unit '%s' in service '%s'", id, desc.Name)
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
	reg.Service.Container.Name = spec.Container.ID
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

func (c proxyConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	//	m["upsql-proxy::proxy-domain"] = svc.ID
	//	m["upsql-proxy::proxy-name"] = u.Name
	//	if len(u.networkings) == 2 && len(u.ports) >= 2 {
	//		adminAddr, dataAddr := "", ""
	//		adminPort, dataPort := 0, 0
	//		for i := range u.networkings {
	//			if u.networkings[i].Type == _ContainersNetworking {
	//				adminAddr = u.networkings[i].IP.String()
	//			} else if u.networkings[i].Type == _ExternalAccessNetworking {
	//				dataAddr = u.networkings[i].IP.String()
	//			}
	//		}

	//		for i := range u.ports {
	//			if u.ports[i].Name == "proxy_data_port" {
	//				dataPort = u.ports[i].Port
	//			} else if u.ports[i].Name == "proxy_admin_port" {
	//				adminPort = u.ports[i].Port
	//				m["adm-cli::proxy_admin_port"] = adminPort
	//			}
	//		}
	//		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", dataAddr, dataPort)
	//		m["adm-cli::adm-cli-address"] = fmt.Sprintf("%s:%d", adminAddr, adminPort)
	//	}

	//	ncpu, err := utils.GetCPUNum(u.config.HostConfig.CpusetCpus)
	//	if err == nil {
	//		m["upsql-proxy::event-threads-count"] = ncpu
	//	} else {
	//		logrus.WithError(err).Warnf("%s upsql-proxy::event-threads-count", u.Name)
	//		m["upsql-proxy::event-threads-count"] = 1
	//	}

	//	swm, err := svc.getSwithManagerUnit()
	//	if err == nil && swm != nil {
	//		swmProxyPort := 0
	//		for i := range swm.ports {
	//			if swm.ports[i].Name == "ProxyPort" {
	//				swmProxyPort = swm.ports[i].Port
	//				break
	//			}
	//		}
	//		if len(swm.networkings) == 1 {
	//			m["adm-cli::adm-svr-address"] = fmt.Sprintf("%s:%d", swm.networkings[0].IP.String(), swmProxyPort)
	//		}
	//	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

func (c proxyConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"/bin/bash"}
	cmds[structs.InitServiceCmd] = []string{"/root/proxy.service", "start"}
	cmds[structs.StartServiceCmd] = []string{"/root/proxy.service", "start"}
	cmds[structs.StopServiceCmd] = []string{"/root/proxy.service", "stop"}

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
	pr := &proxyConfigV102{}
	pr.template = t

	return pr
}

func (c proxyConfigV110) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	//	m["upsql-proxy::proxy-domain"] = svc.ID
	//	m["upsql-proxy::proxy-name"] = u.Name
	//	if len(u.networkings) == 2 && len(u.ports) >= 2 {
	//		adminAddr, dataAddr := "", ""
	//		adminPort, dataPort := 0, 0
	//		for i := range u.networkings {
	//			if u.networkings[i].Type == _ContainersNetworking {
	//				adminAddr = u.networkings[i].IP.String()
	//			} else if u.networkings[i].Type == _ExternalAccessNetworking {
	//				dataAddr = u.networkings[i].IP.String()
	//			}
	//		}

	//		for i := range u.ports {
	//			if u.ports[i].Name == "proxy_data_port" {
	//				dataPort = u.ports[i].Port
	//			} else if u.ports[i].Name == "proxy_admin_port" {
	//				adminPort = u.ports[i].Port
	//				m["adm-cli::proxy_admin_port"] = adminPort
	//			}
	//		}
	//		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", dataAddr, dataPort)
	//		m["supervise::supervise-address"] = fmt.Sprintf("%s:%d", dataAddr, adminPort)
	//		m["adm-cli::adm-cli-address"] = fmt.Sprintf("%s:%d", adminAddr, adminPort)
	//	}

	//	ncpu, err := utils.GetCPUNum(u.config.HostConfig.CpusetCpus)
	//	if err == nil {
	//		m["upsql-proxy::event-threads-count"] = ncpu
	//	} else {
	//		logrus.WithError(err).Warnf("%s upsql-proxy::event-threads-count", u.Name)
	//		m["upsql-proxy::event-threads-count"] = 1
	//	}

	//	swm, err := svc.getSwithManagerUnit()
	//	if err == nil && swm != nil {
	//		swmProxyPort := 0
	//		for i := range swm.ports {
	//			if swm.ports[i].Name == "ProxyPort" {
	//				swmProxyPort = swm.ports[i].Port
	//				break
	//			}
	//		}
	//		if len(swm.networkings) == 1 {
	//			m["adm-cli::adm-svr-address"] = fmt.Sprintf("%s:%d", swm.networkings[0].IP.String(), swmProxyPort)
	//		}
	//	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}
