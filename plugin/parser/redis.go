package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/docker/swarm/garden/structs"
)

func init() {
	register("redis", "v3.5", &redisConfig{})
}

type redisConfig struct {
	config map[string]string
}

func (c redisConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *redisConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		c.config = make(map[string]string, 20)
	}

	c.config[strings.ToLower(key)] = fmt.Sprintf("%v", val)

	return nil
}

func (c *redisConfig) ParseData(data []byte) error {
	if c.config == nil {
		c.config = make(map[string]string, 20)
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
			c.config[string(parts[0])] = string(parts[1])
		}
	}

	return nil
}

func (redisConfig) GenerateConfig(id string, desc structs.ServiceDesc) (map[string]interface{}, error) {
	var (
		m = make(map[string]interface{}, 10)
		//		port = 0
	)
	////	for i := range u.ports {
	////		if u.ports[i].Name == "port" {
	////			port = u.ports[i].Port
	////			break
	////		}
	////	}
	//	m["port"] = port

	//	if len(u.networkings) == 1 {
	//		m["bind"] = u.networkings[0].IP.String()
	//	}

	//	m["maxmemory"] = u.config.HostConfig.Resources.Memory

	return m, nil
}

func (redisConfig) GenerateCommands(id string, desc structs.ServiceDesc) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 4)

	cmds[structs.StartContainerCmd] = []string{"bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/redis.service", "start"}

	cmds[structs.StartContainerCmd] = []string{"/root/redis.service", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/redis.service", "stop"}

	return cmds, nil
}

func (c redisConfig) Marshal() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	for key, val := range c.config {
		_, err := buffer.WriteString(key)
		if err != nil {
			return buffer.Bytes(), err
		}

		err = buffer.WriteByte(' ')
		if err != nil {
			return buffer.Bytes(), err
		}

		_, err = buffer.WriteString(val)
		if err != nil {
			return buffer.Bytes(), err
		}

		err = buffer.WriteByte('\n')
		if err != nil {
			return buffer.Bytes(), err
		}
	}

	return buffer.Bytes(), nil
}

func (redisConfig) Requirement() structs.RequireResource {
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

	return structs.RequireResource{}
}

func (c redisConfig) HealthCheck(id string, desc structs.ServiceDesc) (structs.AgentServiceRegistration, error) {
	//	if c.config == nil {
	//		return healthCheck{}, errors.New("params not ready")
	//	}

	//	addr := c.config["bind"]
	//	port, err := strconv.Atoi(c.config["port"])
	//	if err != nil {
	//		return healthCheck{}, errors.Wrap(err, "get 'Port'")
	//	}

	//	return healthCheck{
	//		Addr: addr,
	//		Port: port,
	//		// Script:   "/opt/DBaaS/script/check_switchmanager.sh " + args[0],
	//		Shell:    "",
	//		Interval: "10s",
	//		TTL:      "",
	//		Tags:     nil,
	//		TCP:      addr + ":" + c.config["port"],
	//	}, nil

	return structs.AgentServiceRegistration{}, nil
}
