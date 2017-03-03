package database

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

type SysConfigOrmer interface {
	InsertSysConfig(c SysConfig) error
	GetSysConfig() (SysConfig, error)
	GetRegistry() (Registry, error)
	GetAuthConfig() (*types.AuthConfig, error)
}

// SysConfig is the application config file
type SysConfig struct {
	ID         int    `db:"dc_id"`
	DockerPort int    `db:"docker_port"`
	PluginPort int    `db:"plugin_port"`
	Retry      int64  `db:"retry"`
	BackupDir  string `db:"backup_dir"`
	ConsulConfig
	HorusConfig
	Registry
	SSHDeliver
	Users
}

// Users is users of DB and Proxy
type Users struct {
	MonitorUsername     string `db:"mon_username"`
	MonitorPassword     string `db:"mon_password"`
	ReplicationUsername string `db:"repl_username"`
	ReplicationPassword string `db:"repl_password"`
	ApplicationUsername string `db:"ap_username"`
	ApplicationPassword string `db:"ap_password"`
	DBAUsername         string `db:"cup_dba_username"`
	DBAPassword         string `db:"cup_dba_password"`
	DBUsername          string `db:"db_username"`
	DBPassword          string `db:"db_password"`
	CheckUsername       string `db:"check_username"`
	CheckPassword       string `db:"check_password"`
}

// SSHDeliver node-init and node-clean settings
type SSHDeliver struct {
	SourceDir       string `db:"source_dir"`
	CA_CRT_Name     string `db:"ca_crt_name"`
	Destination     string `db:"destination_dir"` // must be exist
	InitScriptName  string `db:"init_script_name"`
	CleanScriptName string `db:"clean_script_name"`
}

// DestPath returns destination abs path,pkg\script\CA
func (d SSHDeliver) DestPath() (string, string, string) {
	base := filepath.Base(d.SourceDir)

	return filepath.Join(d.Destination, base, d.InitScriptName),
		filepath.Join(d.Destination, base, d.CA_CRT_Name),
		filepath.Join(d.Destination, d.CleanScriptName)
}

// ConsulConfig consul client config
type ConsulConfig struct {
	ConsulIPs        string `db:"consul_ip"`
	ConsulPort       int    `db:"consul_port"`
	ConsulDatacenter string `db:"consul_dc"`
	ConsulToken      string `db:"consul_token"`
	ConsulWaitTime   int    `db:"consul_wait_time"`
}

// HorusConfig horus config
type HorusConfig struct {
	// HorusServerIP   string `db:"horus_server_ip"`
	// HorusServerPort int    `db:"horus_server_port"`
	HorusAgentPort int `db:"horus_agent_port"`
	//	HorusEventIP   string `db:"horus_event_ip"`
	//	HorusEventPort int    `db:"horus_event_port"`
}

// Registry connection config
type Registry struct {
	OsUsername string `db:"registry_os_username"`
	OsPassword string `db:"registry_os_password"`
	Domain     string `db:"registry_domain"`
	Address    string `db:"registry_ip"`
	Port       int    `db:"registry_port"`
	Username   string `db:"registry_username"`
	Password   string `db:"registry_password"`
	Email      string `db:"registry_email"`
	Token      string `db:"registry_token"`
	CA_CRT     string `db:"registry_ca_crt"`
}

func (db dbBase) sysConfigTable() string {
	return db.prefix + "_system_config"
}

// InsertSysConfig insert a new SysConfig
func (db dbBase) InsertSysConfig(c SysConfig) error {

	query := "INSERT INTO " + db.sysConfigTable() + " (dc_id,consul_ip,consul_port,consul_dc,consul_token,consul_wait_time,horus_agent_port,registry_domain,registry_ip,registry_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt,source_dir,clean_script_name,init_script_name,ca_crt_name,destination_dir,docker_port,plugin_port,retry,registry_os_username,registry_os_password,mon_username,mon_password,repl_username,repl_password,cup_dba_username,cup_dba_password,db_username,db_password,ap_username,ap_password,check_username,check_password,backup_dir) VALUES (:dc_id,:consul_ip,:consul_port,:consul_dc,:consul_token,:consul_wait_time,:horus_agent_port,:registry_domain,:registry_ip,:registry_port,:registry_username,:registry_password,:registry_email,:registry_token,:registry_ca_crt,:source_dir,:clean_script_name,:init_script_name,:ca_crt_name,:destination_dir,:docker_port,:plugin_port,:retry,:registry_os_username,:registry_os_password,:mon_username,:mon_password,:repl_username,:repl_password,:cup_dba_username,:cup_dba_password,:db_username,:db_password,:ap_username,:ap_password,:check_username,:check_password,:backup_dir)"

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
	var (
		c     = SysConfig{}
		query = "SELECT dc_id,consul_ip,consul_port,consul_dc,consul_token,consul_wait_time,horus_agent_port,registry_domain,registry_ip,registry_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt,source_dir,clean_script_name,init_script_name,ca_crt_name,destination_dir,docker_port,plugin_port,retry,registry_os_username,registry_os_password,mon_username,mon_password,repl_username,repl_password,cup_dba_username,cup_dba_password,db_username,db_password,ap_username,ap_password,check_username,check_password,backup_dir FROM " + db.sysConfigTable() + " LIMIT 1"
	)

	err := db.Get(c, query)

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
	var (
		r     Registry
		query = "SELECT registry_domain,registry_ip,registry_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt FROM " + db.sysConfigTable() + " LIMIT 1"
	)

	err := db.Get(&r, query)
	if err != nil {
		return r, errors.Wrap(err, "get SysConfig.Registry")
	}

	return r, nil
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
