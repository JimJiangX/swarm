package structs

import "github.com/samalba/dockerclient"

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

	Config dockerclient.ContainerConfig

	StoreType string
	StoreSize int64
}

func (m Module) Store() (string, int64) {

	return m.StoreType, m.StoreSize

}
