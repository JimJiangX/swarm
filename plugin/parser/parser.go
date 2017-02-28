package parser

import (
	"fmt"
	"sync"

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

var images = struct {
	sync.RWMutex
	m map[string]structs.ImageVersion
}{
	m: make(map[string]structs.ImageVersion, 10),
}

func register(name, version string, _ parser) error {
	key := name + ":" + version

	images.RLock()
	_, exist := images.m[key]
	images.RUnlock()

	if exist {
		return fmt.Errorf("image:%s exist", key)
	}

	v, err := structs.ParseImage(key)
	if err != nil {
		return err
	}

	images.Lock()
	images.m[key] = v
	images.Unlock()

	return nil
}

func factory(name, version string) (parser, error) {
	switch {
	default:
	}

	return nil, fmt.Errorf("Unsupported image %s:%s yet.", name, version)
}
