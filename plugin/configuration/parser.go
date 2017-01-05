package main

import (
	"github.com/docker/swarm/garden/structs"
)

type parser interface {
	Validate(data map[string]interface{}) error

	ParseData(data []byte) error

	GenerateConfig(args ...interface{}) (structs.ConfigCmds, error)

	GenerateCommands() (structs.CmdsMap, error)

	marshal() (string, error)

	Requirement() interface{}
}
