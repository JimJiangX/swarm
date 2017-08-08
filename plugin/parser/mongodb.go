package parser

import (
	"github.com/docker/swarm/garden/structs"
)

func init() {
	register("mongodb", "3.4", &mongodbConfig{})
}

type mongodbConfig struct {
	template *structs.ConfigTemplate
}

func (mongodbConfig) clone(t *structs.ConfigTemplate) parser {
	return &mongodbConfig{
		template: t,
	}
}

func (mongodbConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (mongodbConfig) ParseData(data []byte) error {
	return nil
}

func (mongodbConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	return nil
}

func (mongodbConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	return nil, nil
}

func (mongodbConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
	return structs.ServiceRegistration{}, nil
}

func (mongodbConfig) Marshal() ([]byte, error) {
	return nil, nil
}
