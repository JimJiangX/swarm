package gardener

import "github.com/docker/engine-api/types/container"

type PostCreateService struct {
	Name    string
	Modules []Module
}

type Module struct {
	Name       string
	Version    string
	Type       string
	Arch       string // split by `-` ,"nMaster-mStandby-xSlave"
	Nodes      []string
	Configures map[string]interface{}

	Config     container.Config
	HostConfig container.HostConfig
}
