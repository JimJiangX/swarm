package structs

import (
	"github.com/docker/swarm/garden/database"
	"github.com/hashicorp/consul/api"
)

const (
	StartContainerCmd = "start_container_cmd"
	InitServiceCmd    = "init_service_cmd"
	StartServiceCmd   = "start_service_cmd"
	StopServiceCmd    = "stop_service_cmd"
	RestoreCmd        = "restore_cmd"
	BackupCmd         = "backup_cmd"
	HealthCheckCmd    = "health_check_cmd"
)

type HorusRegistration struct {
	Endpoint      string
	CollectorName string   `json:"collectorname,omitempty"`
	User          string   `json:"user,omitempty"`
	Password      string   `json:"pwd,omitempty"`
	Type          string   `json:"type"`
	CollectorIP   string   `json:"colletorip"`   // spell error
	CollectorPort int      `json:"colletorport"` // spell error
	MetricTags    string   `json:"metrictags"`
	Network       []string `json:"network,omitempty"`
	Status        string   `json:"status"`
	Table         string   `json:"table"`
	CheckType     string   `json:"checktype"`
}

type ServiceRegistration struct {
	Consul AgentServiceRegistration
	Horus  HorusRegistration
}

type AgentServiceRegistration api.AgentServiceRegistration

type ConfigCmds struct {
	ID        string
	Path      string
	Name      string
	Version   string
	Context   string
	Cmds      CmdsMap
	Timestamp int64

	Registration ServiceRegistration
}

type Keyset struct {
	Key    string
	CanSet bool
	Desc   string
}

type RequireResource struct {
	IPs []struct {
		Name  string
		IP    string
		Proto string
	}
	Ports []struct {
		Name string
		Port int
	}
}

type ConfigTemplate struct {
	Name      string
	Version   string
	Path      string
	Context   []byte
	Keysets   []Keyset
	Timestamp int64
}

type UnitResources struct {
	ID, Name string

	Require, Limit struct {
		CPU    string
		Memory int64
	}

	Engine struct {
		ID   string
		Name string
		IP   string
	}

	Networking struct {
		Type    string
		Devices string
		Mask    int
		IPs     []struct {
			Name  string
			IP    string
			Proto string
		}
		Ports []struct {
			Name string
			Port int
		}
	}

	Volumes []struct {
		Type    string
		Driver  string
		Size    int
		Options map[string]interface{}
	}
}

type ServiceDesc struct {
	ID      string
	Name    string
	Arch    string
	Image   string
	Version string

	Options map[string]interface{}

	Users []database.User

	Units []UnitResources

	Dependent []*ServiceDesc
}

type CmdsMap map[string][]string

type Commands map[string]CmdsMap

type ConfigsMap map[string]ConfigCmds

func (c CmdsMap) Get(typ string) []string {
	if c == nil {
		return nil
	}

	return c[typ]
}

func (c ConfigCmds) GetCmd(typ string) []string {
	if c.Cmds == nil {
		return nil
	}

	return c.Cmds.Get(typ)
}

func (c ConfigCmds) GetServiceRegistration() ServiceRegistration {
	return c.Registration
}

func (c Commands) GetCmd(key, typ string) []string {
	if cmds, ok := c[key]; ok {
		return cmds.Get(typ)
	}

	return nil
}

func (m ConfigsMap) Get(key string) (ConfigCmds, bool) {
	if m == nil {
		return ConfigCmds{}, false
	}

	val, ok := m[key]

	return val, ok
}

func (m ConfigsMap) GetCmd(key, typ string) []string {
	val, ok := m.Get(key)
	if ok {
		return val.GetCmd(typ)
	}

	return nil
}
