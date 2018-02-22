package structs

import "github.com/hashicorp/consul/api"

const (
	StartContainerCmd = "start_container_cmd"
	InitServiceCmd    = "init_service_cmd"
	StartServiceCmd   = "start_service_cmd"
	StopServiceCmd    = "stop_service_cmd"
	RestoreCmd        = "restore_cmd"
	BackupCmd         = "backup_cmd"
	HealthCheckCmd    = "health_check_cmd"
	MigrateRebuildCmd = "migrate_rebuild_cmd"
)

//type HorusRegistration2 struct {
//	Endpoint      string   `json:"endpoint"`
//	CollectorName string   `json:"collectorname,omitempty"`
//	User          string   `json:"user,omitempty"`
//	Password      string   `json:"pwd,omitempty"`
//	Type          string   `json:"type"`
//	CollectorIP   string   `json:"colletorip"`   // spell error
//	CollectorPort int      `json:"colletorport"` // spell error
//	MetricTags    string   `json:"metrictags"`
//	Network       []string `json:"network,omitempty"`
//	Status        string   `json:"status"`
//	Table         string   `json:"table"`
//	CheckType     string   `json:"checktype"`
//}

type HorusRegistration struct {
	Node struct {
		Select     bool     `json:"select"`
		Name       string   `json:"name"`
		IPAddr     string   `json:"ip_addr"`
		Port       string   `json:"ssh_port"`
		OSUser     string   `json:"os_user"`
		OSPassword string   `json:"os_pwd"`
		CheckType  string   `json:"check_type"`
		NetDevice  []string `json:"net_dev"`
	} `json:"node,omitempty"`

	Service struct {
		Select          bool   `json:"select"`
		Name            string `json:"name"`
		Type            string `json:"type"`
		MonitorUser     string `json:"mon_user"`
		MonitorPassword string `json:"mon_pwd"`
		Tag             string `json:"tag"`

		Container struct {
			Name     string `json:"name"`
			HostName string `json:"host_name"`
		} `json:"container,omitempty"`
	} `json:"service,omitempty"`
}

type ServiceRegistration struct {
	Consul *api.AgentServiceRegistration `json:"consul_server,omitempty"`
	Horus  *HorusRegistration            `json:"horus_server,omitempty"`
}

type ServiceDeregistration struct {
	Type     string
	Key      string
	Addr     string
	Port     string `json:"ssh_port"`
	User     string
	Password string
}

type ConfigCmds struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Version    string  `json:"version"`
	Content    string  `json:"content"`
	LogMount   string  `json:"log_mount"`
	DataMount  string  `json:"data_mount"`
	ConfigFile string  `json:"config_file"`
	Cmds       CmdsMap `json:"cmds,omitempty"`
	Timestamp  int64   `json:"timestamp"`

	Registration ServiceRegistration `json:"registration,omitempty"`
}

type CmdsMap map[string][]string

type Commands map[string]CmdsMap

type ConfigsMap map[string]ConfigCmds

type ServiceConfigs []UnitConfig

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

func (c ConfigsMap) Commands() Commands {
	if len(c) == 0 {
		return nil
	}

	cm := make(Commands)

	for key, val := range c {
		cm[key] = val.Cmds
	}

	return cm
}
