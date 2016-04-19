package database

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

type Configurations struct {
	ID int `db:"id"`
	ConsulConfig
	HorusConfig
	Registry
	SSHDeliver

	DockerPort int  `db:"docker_port"`
	PluginPort int  `db:"plugin_port"`
	Retry      byte `db:"retry"`
}

type SSHDeliver struct {
	SourceDir   string `db:"source_dir"`
	PkgName     string `db:"pkg_name"`
	ScriptName  string `db:"script_name"`
	CA_CRT_Name string `db:"ca_crt_name"`
	Destination string `db:"destination_dir"` // must be exist
}

// DestPath returns destination abs path,pkg\script\CA
func (d SSHDeliver) DestPath() (string, string, string) {
	base := filepath.Base(d.SourceDir)

	return filepath.Join(d.Destination, d.PkgName),
		filepath.Join(d.Destination, base, d.ScriptName),
		filepath.Join(d.Destination, base, d.CA_CRT_Name)
}

type ConsulConfig struct {
	ConsulIPs        string `db:"consul_IPs"`
	ConsulPort       int    `db:"consul_port"`
	ConsulDatacenter string `db:"consul_dc"`
	ConsulToken      string `db:"consul_token"`
	ConsulWaitTime   int    `db:"consul_wait_time"`
}

type HorusConfig struct {
	HorusDistributionIP   string `db:"horus_distribution_ip"`
	HorusDistributionPort int    `db:"horus_distribution_port"`
	HorusEventIP          string `db:"horus_event_ip"`
	HorusEventPort        int    `db:"horus_event_port"`
}

type Registry struct {
	OsUsername string `db:"registry_os_username"`
	OsPassword string `db:"registry_os_password"`
	Domain     string `db:"registry_domain"`
	Address    string `db:"registry_address"`
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

func (c *Configurations) Insert() (int64, error) {
	query := "INSERT INTO tb_system_config (consul_IPs,consul_port,consul_dc,consul_token,consul_wait_time,horus_distribution_ip,horus_distribution_port,horus_event_ip,horus_event_port,registry_domain,registry_address,registry_port,registry_username,registry_password,registry_email,registry_token,registry_ca_crt,source_dir,pkg_name,script_name,ca_crt_name,destination_dir,docker_port,plugin_port,retry) VALUES (:consul_IPs,:consul_port,:consul_dc,:consul_token,:consul_wait_time,:horus_distribution_ip,:horus_distribution_port,:horus_event_ip,:horus_event_port,:registry_domain,:registry_address,:registry_port,:registry_username,:registry_password,:registry_email,:registry_token,:registry_ca_crt,:source_dir,:pkg_name,:script_name,:ca_crt_name,:destination_dir,:docker_port,:plugin_port,:retry)"
	db, err := GetDB(true)
	if err != nil {
		return 0, err
	}

	r, err := db.NamedExec(query, c)
	if err != nil {
		return 0, err
	}

	return r.LastInsertId()
}
func (c Configurations) GetConsulClient() ([]*consulapi.Client, error) {
	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ",")
	clients := make([]*consulapi.Client, 0, len(addrs))

	for i := range addrs {
		config := consulapi.Config{
			Address:    fmt.Sprintf("%s:%d", addrs[i], port),
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

func (c Configurations) GetConsulConfig() ([]string, string, string, int) {
	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ",")
	endpoints := make([]string, len(addrs))

	for i := range addrs {
		endpoints[i] = addrs[i] + ":" + port
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
	err = db.QueryRowx("SELECT * FROM tb_system_config LIMIT 1").StructScan(c)
	if err != nil {
		return nil, err
	}

	return c, nil
}
