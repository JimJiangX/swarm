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

func (d SSHDeliver) DestPath() (string, string, string) {
	return filepath.Join(d.Destination, d.PkgName),
		filepath.Join(d.Destination, d.ScriptName),
		filepath.Join(d.Destination, d.CA_CRT_Name)
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
	HorusDistributionPort int    `db:"horus_distribution_port`
	HorusEventIP          string `db:"horus_event_ip`
	HorusEventPort        int    `db:"Horus_enevt_port`
}

type Registry struct {
	Domain        string `db:"registry_domain"`
	Address       string `db:"registry_address"`
	RegistryPort  int    `db:"registry_port"`
	Username      string `db:"registry_username"`
	Password      string `db:"registry_password"`
	Email         string `db:"registry_email"`
	RegistryToken string `db:"registry_token"`
	CA_CRT        string `db:"registry_ca_crt"`
}

func (c Configurations) TableName() string {
	return "tb_system_config"
}

func (c Configurations) GetConsulClient() ([]*consulapi.Client, error) {
	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ";&;")

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
	addrs := strings.Split(c.ConsulIPs, ";&;")

	endpoints := make([]string, len(addrs))

	for i := range addrs {
		endpoints[i] = addrs[i] + ":" + port
	}

	return endpoints, c.ConsulDatacenter, c.ConsulToken, c.ConsulWaitTime
}

func (c Configurations) GetConsulAddrs() []string {
	return strings.Split(c.ConsulIPs, ";&;")
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

	return c, err
}
