package parser

import (
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

const (
	imageKey  = "/images"
	configKey = "/configs"
)

type parser interface {
	clone(*structs.ConfigTemplate) parser

	set(key string, val interface{}) error
	get(key string) string

	Validate(data map[string]interface{}) error

	ParseData(data []byte) error

	GenerateConfig(id string, desc structs.ServiceSpec) error

	GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error)

	HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error)

	Marshal() ([]byte, error)
}

var images = make(map[structs.ImageVersion]parser, 10)

func register(name, version string, pr parser) error {

	v, err := structs.ParseImage(name + ":" + version)
	if err != nil {
		return err
	}

	images[v] = pr

	return nil
}

func factory(name string) (parser, error) {
	im, err := structs.ParseImage(name)
	if err != nil {
		return nil, err
	}

	if pr, ok := images[im]; ok && pr != nil {
		return pr, nil
	}

	var (
		temp parser
		less = 0xFF
	)

	for iv, pr := range images {
		if iv.Name == im.Name && iv.Major == im.Major && iv.Minor == im.Minor {
			if n := im.Patch - iv.Patch; n >= 0 && n < less {
				temp = pr
				less = n
			}
		}
	}

	if temp != nil {
		return temp, nil
	}

	return nil, errors.Errorf("Unsupported image %s yet", name)
}
