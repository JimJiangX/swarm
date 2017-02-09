package parser

import (
	"fmt"

	"github.com/docker/swarm/garden/structs"
)

const (
	imageKey  = "image"
	configKey = "config"
)

type parser interface {
	Validate(data map[string]interface{}) error

	Set(key string, val interface{}) error

	ParseData(data []byte) error

	GenerateConfig(id string, desc structs.ServiceSpec) error

	GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error)

	HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error)

	Marshal() ([]byte, error)

	Requirement() structs.RequireResource
}

var images = make(map[string]bool, 10)

func register(name, version string, _ parser) error {
	key := name + ":" + version
	if _, exist := images[key]; exist {
		return fmt.Errorf("image:%s exist", key)
	}

	images[key] = true

	return nil
}

func factory(name, version string) (parser, error) {
	switch {
	default:
	}

	return nil, fmt.Errorf("Unsupported image %s:%s yet.", name, version)
}
