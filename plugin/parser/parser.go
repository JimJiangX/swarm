package parser

import (
	"fmt"

	"github.com/docker/swarm/garden/structs"
)

const (
	prefix    = "upmio/docker/swarm"
	imageKey  = "image"
	configKey = "config"
)

type parser interface {
	Validate(data map[string]interface{}) error

	Set(key string, val interface{}) error

	ParseData(data []byte) error

	GenerateConfig(id string, desc structs.ServiceDesc) (map[string]interface{}, error)

	GenerateCommands(id string, desc structs.ServiceDesc) (structs.CmdsMap, error)

	HealthCheck(id string, desc structs.ServiceDesc) (structs.AgentServiceRegistration, error)

	Marshal() ([]byte, error)

	Requirement() structs.RequireResource
}

var images = make(map[string]bool, 10)

func register(name, version string, _ parser) error {
	key := name + ":" + version
	if _, exist := images[key]; exist {
		return fmt.Errorf("image:%s:%s exist", key)
	}

	images[key] = true

	return nil
}
