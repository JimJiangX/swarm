package database

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

type GetSysConfigIface interface {
	GetSysConfig() (SysConfig, error)
}

type SysConfigOrmer interface {
	GetSysConfigIface

	InsertSysConfig(c SysConfig) error
	GetPorts() (Ports, error)
	GetRegistry() (Registry, error)
	GetAuthConfig() (*types.AuthConfig, error)
}

// SysConfig is the application config file
type SysConfig struct {
	ID        int    `db:"dc_id" json:"dc_id"`
	Retry     int64  `db:"retry" json:"retry"`
	BackupDir string `db:"backup_dir" json:"backup_dir"`
	Ports
	ConsulConfig
	Registry
	SSHDeliver
}

type Ports struct {
	Docker int `db:"docker_port" json:"docker_port"`
	// Plugin     int `db:"plugin_port" json:"plugin_port"`
	SwarmAgent int `db:"swarm_agent_port" json:"swarm_agent_port"`
	// Consul     int `db:"consul_port"`
}

// SSHDeliver node-init and node-clean settings
type SSHDeliver struct {
	SourceDir       string `db:"source_dir" json:"source_dir"`
	CACertName      string `db:"ca_crt_name" json:"-"`
	Destination     string `db:"destination_dir" json:"destination_dir"` // must be exist
	InitScriptName  string `db:"init_script_name" json:"init_script_name"`
	CleanScriptName string `db:"clean_script_name" json:"clean_script_name"`
}

// DestPath returns destination abs path,pkg\CA\script
func (d SSHDeliver) DestPath() (string, string, string) {
	base := filepath.Base(d.SourceDir)

	return filepath.Join(d.Destination, base, d.InitScriptName),
		filepath.Join(d.Destination, base, d.CACertName),
		filepath.Join(d.Destination, d.CleanScriptName)
}

// ConsulConfig consul client config
type ConsulConfig struct {
	ConsulIPs        string `db:"consul_ip" json:"consul_ip"`
	ConsulPort       int    `db:"consul_port" json:"consul_port"`
	ConsulDatacenter string `db:"consul_dc" json:"consul_dc"`
	ConsulToken      string `db:"consul_token" json:"-"`
	ConsulWaitTime   int    `db:"consul_wait_time" json:"consul_wait_time"`
}

// HorusConfig horus config
type HorusConfig struct {
	// HorusServerIP   string `db:"horus_server_ip"`
	// HorusServerPort int    `db:"horus_server_port"`
	// HorusAgentPort int `db:"horus_agent_port"`
	// HorusEventIP   string `db:"horus_event_ip"`
	// HorusEventPort int    `db:"horus_event_port"`
}

// Registry connection config
type Registry struct {
	OsUsername string `db:"registry_os_username" json:"-"`
	// OsPassword string `db:"registry_os_password" json:"-"`
	Domain   string `db:"registry_domain" json:"registry_domain"`
	Address  string `db:"registry_ip" json:"registry_ip"`
	Port     int    `db:"registry_port" json:"registry_port"`
	SSHPort  int    `db:"registry_ssh_port" json:"registry_ssh_port"`
	Username string `db:"registry_username" json:"-"`
	Password string `db:"registry_password" json:"-"`
	Email    string `db:"registry_email" json:"-"`
	Token    string `db:"registry_token" json:"-"`
	CACert   string `db:"registry_ca_crt" json:"-"`
}

func (db dbBase) sysConfigTable() string {
	return db.prefix + "_system_config"
}

// InsertSysConfig insert a new SysConfig
func (db dbBase) InsertSysConfig(c SysConfig) error {

	query := "INSERT INTO " + db.sysConfigTable() + " (dc_id,consul_ip,consul_port,consul_dc,consul_token,consul_wait_time,swarm_agent_port,registry_os_username,registry_domain,registry_ip,registry_port,registry_ssh_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt,source_dir,clean_script_name,init_script_name,ca_crt_name,destination_dir,docker_port,retry,backup_dir) VALUES (:dc_id,:consul_ip,:consul_port,:consul_dc,:consul_token,:consul_wait_time,:swarm_agent_port,:registry_os_username,:registry_domain,:registry_ip,:registry_port,:registry_ssh_port,:registry_username,:registry_password,:registry_email,:registry_token,:registry_ca_crt,:source_dir,:clean_script_name,:init_script_name,:ca_crt_name,:destination_dir,:docker_port,:retry,:backup_dir)"

	_, err := db.NamedExec(query, &c)

	return errors.Wrap(err, "insert SysConfig")
}

// GetConsulConfig returns consul config
func (c SysConfig) GetConsulConfig() ([]string, string, string, int) {
	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ",")
	endpoints := make([]string, 0, len(addrs)+1)

	for i := range addrs {
		endpoints = append(endpoints, addrs[i]+":"+port)
	}

	return endpoints, c.ConsulDatacenter, c.ConsulToken, c.ConsulWaitTime
}

// GetConsulAddrs returns consul IP
func (c SysConfig) GetConsulAddrs() []string {
	return strings.Split(c.ConsulIPs, ",")
}

// GetSysConfig returns *SysConfig
func (db dbBase) GetSysConfig() (SysConfig, error) {
	var c = SysConfig{}

	query := "SELECT dc_id,consul_ip,consul_port,consul_dc,consul_token,consul_wait_time,swarm_agent_port,registry_domain,registry_ip,registry_port,registry_ssh_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt,source_dir,clean_script_name,init_script_name,ca_crt_name,destination_dir,docker_port,retry,registry_os_username,backup_dir FROM " + db.sysConfigTable() + " LIMIT 1"

	err := db.Get(&c, query)

	return c, errors.Wrap(err, "get SystemConfig")
}

func (db dbBase) GetAuthConfig() (*types.AuthConfig, error) {
	r, err := db.GetRegistry()
	if err != nil {
		return nil, err
	}

	return r.AuthConfig(), nil
}

func (db dbBase) GetRegistry() (Registry, error) {
	var r Registry

	query := "SELECT registry_os_username,registry_domain,registry_ip,registry_port,registry_ssh_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt FROM " + db.sysConfigTable() + " LIMIT 1"

	err := db.Get(&r, query)

	return r, errors.Wrap(err, "get SysConfig.Registry")
}

func (r Registry) AuthConfig() *types.AuthConfig {
	return newAuthConfig(r.Username, r.Password, r.Email, r.Token)
}

func newAuthConfig(username, password, email, token string) *types.AuthConfig {
	return &types.AuthConfig{
		Username:      username,
		Password:      password,
		Email:         email,
		RegistryToken: token,
	}
}

func (db dbBase) GetPorts() (Ports, error) {
	var p Ports

	query := "SELECT swarm_agent_port,docker_port FROM " + db.sysConfigTable() + " LIMIT 1"

	err := db.Get(&p, query)

	return p, errors.Wrap(err, "get sysConfig.Ports")
}
