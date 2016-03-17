package gardener

import "github.com/docker/swarm/cluster"

type PostServiceRequest struct {
	Name    string
	Modules []Module
}

type Module struct {
	Name       string
	Version    string
	Type       string
	Arch       string // split by `-` ,"nMaster-mStandby-xSlave"
	Num        int
	Nodes      []string
	Configures map[string]interface{}

	Config cluster.ContainerConfig
}
