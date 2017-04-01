package parser

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

func init() {
	register("redis", "3.2", &redisConfig{})
}

type redisConfig struct {
	template *structs.ConfigTemplate
	config   map[string]string
}

func (redisConfig) clone(t *structs.ConfigTemplate) parser {
	return &redisConfig{
		template: t,
		config:   make(map[string]string, 100),
	}
}

func (c redisConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *redisConfig) ParseData(data []byte) error {
	if c.config == nil {
		c.config = make(map[string]string, 100)
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

func (c redisConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	var spec *structs.UnitSpec

	for i := range desc.Units {
		if id == desc.Units[i].ID {
			spec = &desc.Units[i]
			break
		}
	}

	if spec == nil {
		return errors.Errorf("not found unit '%s' in service '%s'", id, desc.Name)
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

	cmds[structs.InitServiceCmd] = []string{"/root/redis.service", "start"}

	cmds[structs.StartServiceCmd] = []string{"/root/redis.service", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/redis.service", "stop"}

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

//func (redisConfig) Requirement() structs.RequireResource {
//	ports := []port{
//		port{
//			proto: "tcp",
//			name:  "port",
//		},
//	}
//	nets := []netRequire{
//		netRequire{
//			Type: _ContainersNetworking,
//			num:  1,
//		},
//	}
//	return require{
//		ports:       ports,
//		networkings: nets,
//	}

//	return structs.RequireResource{}
//}

func (c redisConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
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

	//	Service struct {
	//		Select bool `json:"-"`

	//		Name            string
	//		Type            string
	//		MonitorUser     string `json:"mon_user"`
	//		MonitorPassword string `json:"mon_pwd"`
	//		Tag             string

	//		Container struct {
	//			Name     string
	//			HostName string `json:"host_name"`
	//		} `json:"container"`
	//	}

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

	//	var mon *structs.User

	//	if len(desc.Users) > 0 {
	//		for i := range desc.Users {
	//			if desc.Users[i].Role == "mon" {
	//				mon = &desc.Users[i]
	//				break
	//			}
	//		}

	//		if mon != nil {
	//			reg.Service.MonitorUser = mon.Name
	//			reg.Service.MonitorPassword = mon.Password
	//		}
	//	}

	return structs.ServiceRegistration{Horus: &reg}, nil
}
