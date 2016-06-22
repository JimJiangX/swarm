package database

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

type Configurations struct {
	ID         int    `db:"dc_id"`
	DockerPort int    `db:"docker_port"`
	PluginPort int    `db:"plugin_port"`
	Retry      int64  `db:"retry"`
	BackupDir  string `db:"backup_dir"`
	NFSOption
	ConsulConfig
	HorusConfig
	Registry
	SSHDeliver
	Users
}

type NFSOption struct {
	Addr         string `db:"nfs_ip"`
	Dir          string `db:"nfs_dir"`
	MountDir     string `db:"nfs_mount_dir"`
	MountOptions string `db:"nfs_mount_opts"`
}
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
}

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

type ConsulConfig struct {
	ConsulIPs        string `db:"consul_ip"`
	ConsulPort       int    `db:"consul_port"`
	ConsulDatacenter string `db:"consul_dc"`
	ConsulToken      string `db:"consul_token"`
	ConsulWaitTime   int    `db:"consul_wait_time"`
}

type HorusConfig struct {
	HorusServerIP   string `db:"horus_server_ip"`
	HorusServerPort int    `db:"horus_server_port"`
	HorusAgentPort  int    `db:"horus_agent_port"`
	HorusEventIP    string `db:"horus_event_ip"`
	HorusEventPort  int    `db:"horus_event_port"`
}

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

func (c Configurations) TableName() string {
	return "tb_system_config"
}

func (c Configurations) Insert() (int64, error) {
	query := "INSERT INTO tb_system_config (dc_id,backup_dir,consul_ip,consul_port,consul_dc,consul_token,consul_wait_time,horus_server_ip,horus_server_port,horus_agent_port,horus_event_ip,horus_event_port,registry_domain,registry_ip,registry_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt,source_dir,clean_script_name,init_script_name,ca_crt_name,destination_dir,docker_port,plugin_port,retry,registry_os_username,registry_os_password,mon_username,mon_password,repl_username,repl_password,cup_dba_username,cup_dba_password,db_username,db_password,ap_username,ap_password,nfs_ip,nfs_dir,nfs_mount_dir,nfs_mount_opts) VALUES (:dc_id,:backup_dir,:consul_ip,:consul_port,:consul_dc,:consul_token,:consul_wait_time,:horus_server_ip,:horus_server_port,:horus_agent_port,:horus_event_ip,:horus_event_port,:registry_domain,:registry_ip,:registry_port,:registry_username,:registry_password,:registry_email,:registry_token,:registry_ca_crt,:source_dir,:clean_script_name,:init_script_name,:ca_crt_name,:destination_dir,:docker_port,:plugin_port,:retry,:registry_os_username,:registry_os_password,:mon_username,:mon_password,:repl_username,:repl_password,:cup_dba_username,:cup_dba_password,:db_username,:db_password,:ap_username,:ap_password,:nfs_ip,:nfs_dir,:nfs_mount_dir,:nfs_mount_opts)"
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	r, err := db.NamedExec(query, &c)
	if err != nil {
		return 0, err
	}

	return r.LastInsertId()
}

func (c Configurations) GetConsulClient() ([]*consulapi.Client, error) {
	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ",")
	clients := make([]*consulapi.Client, 0, len(addrs)+1)

	for i := range addrs {
		config := consulapi.Config{
			Address:    addrs[i] + ":" + port,
			Datacenter: c.ConsulDatacenter,
			WaitTime:   time.Duration(c.ConsulWaitTime) * time.Second,
			Token:      c.ConsulToken,
		}

		client, err := consulapi.NewClient(&config)
		if err == nil {
			clients = append(clients, client)
		}
	}

	return clients, nil
}

func (c Configurations) GetConsulConfigs() []consulapi.Config {
	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ",")
	if len(addrs) == 0 {
		return nil
	}
	configs := make([]consulapi.Config, len(addrs))

	for i := range addrs {
		configs[i] = consulapi.Config{
			Address:    addrs[i] + ":" + port,
			Datacenter: c.ConsulDatacenter,
			WaitTime:   time.Duration(c.ConsulWaitTime) * time.Second,
			Token:      c.ConsulToken,
		}
	}

	return configs
}

func (c Configurations) GetConsulConfig() ([]string, string, string, int) {
	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ",")
	endpoints := make([]string, 0, len(addrs)+1)

	for i := range addrs {
		endpoints = append(endpoints, addrs[i]+":"+port)
	}

	return endpoints, c.ConsulDatacenter, c.ConsulToken, c.ConsulWaitTime
}

func (c Configurations) GetConsulAddrs() []string {
	return strings.Split(c.ConsulIPs, ",")
}

func GetSystemConfig() (*Configurations, error) {
	db, err := GetDB(true)
	if err != nil {
		return nil, err
	}

	c := &Configurations{}
	err = db.Get(c, "SELECT * FROM tb_system_config LIMIT 1")
	if err != nil {
		return nil, err
	}

	return c, nil
}

func deleteSystemConfig(id int64) error {
	db, err := GetDB(true)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tb_system_config WHERE dc_id=?", id)

	return err
}
