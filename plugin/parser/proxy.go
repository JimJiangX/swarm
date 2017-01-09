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
	register("proxy", "v1.0", &proxyConfig{})
	register("proxy", "v1.0.2", &proxyConfigV102{})
	register("proxy", "v1.1.0", &proxyConfigV110{})
}

type proxyConfig struct {
	config config.Configer
}

func (proxyConfig) Validate(data map[string]interface{}) error { return nil }

func (c *proxyConfig) Set(key string, val interface{}) error {
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
		return nil, err
	}

	data, err := ioutil.ReadFile(tmpfile.Name())

	return data, errors.Wrap(err, "read file")
}

func (proxyConfig) Requirement() structs.RequireResource {
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

	return structs.RequireResource{}
}

func (c proxyConfig) HealthCheck(id string, desc structs.ServiceDesc) (structs.ServiceRegistration, error) {
	//	if c.config == nil || len(args) == 0 {
	//		return healthCheck{}, errors.New("params not ready")
	//	}

	//	addr := c.config.String("adm-cli::adm-cli-address")
	//	port, err := c.config.Int("adm-cli::proxy_admin_port")
	//	if err != nil {
	//		return healthCheck{}, errors.Wrap(err, "get 'adm-cli::proxy_admin_port'")
	//	}

	//	return healthCheck{
	//		Addr:     addr,
	//		Port:     port,
	//		Script:   "/opt/DBaaS/script/check_proxy.sh " + args[0],
	//		Shell:    "",
	//		Interval: "10s",
	//		TTL:      "",
	//		Tags:     nil,
	//	}, nil

	return structs.ServiceRegistration{}, nil
}

func (c proxyConfig) GenerateConfig(id string, desc structs.ServiceDesc) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	for key, val := range desc.Options {
		_ = key
		_ = val
	}

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
		err = c.Set(key, val)
	}

	return err
}

func (c proxyConfig) GenerateCommands(id string, desc structs.ServiceDesc) (structs.CmdsMap, error) {
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

type proxyConfigV110 struct {
	proxyConfig
}

func (c proxyConfigV110) GenerateConfig(id string, desc structs.ServiceDesc) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	m := make(map[string]interface{}, 10)

	for key, val := range desc.Options {
		_ = key
		_ = val
	}
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
		err = c.Set(key, val)
	}

	return err
}
