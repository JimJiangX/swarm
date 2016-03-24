package database

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

type Configurations struct {
	ID                    int    `db:"id"`
	ConsulIPs             string `db:"consul_IPs"`
	ConsulPort            int    `db:"consul_port"`
	ConsulDatacenter      string `db:"consul_dc"`
	ConsulToken           string `db:"consul_token"`
	ConsulWaitTime        int    `db:"consul_wait_time"`
	DockerPort            int    `db:"docker_port"`
	HorusDistributionIP   string `db:"horus_distribution_ip"`
	HorusDistributionPort int    `db:"horus_distribution_port`
	HorusEventIP          string `db:"horus_event_ip`
	HorusEventPort        int    `db:"Horus_enevt_port`
	SwarmManageEndpoints  string `db:"swarm_m_endpoints`
	Retry                 byte   `db:"retry"`
}

func (c Configurations) TableName() string {
	return "tb_system_config"
}

func GetConsulClient() []*consulapi.Client {
	// query data
	c, err := GetConsulConfigFromDB()

	port := strconv.Itoa(c.ConsulPort)
	addrs := strings.Split(c.ConsulIPs, ";&;")

	clients := make([]*consulapi.Client, len(addrs))

	for i := range addrs {
		config := consulapi.Config{
			Address:    fmt.Sprintf("%s:%d", addrs[i], port),
			Datacenter: c.ConsulDatacenter,
			WaitTime:   time.Duration(c.ConsulWaitTime) * time.Second,
			Token:      c.ConsulToken,
		}

		clients[i], err = consulapi.NewClient(&config)
		if err != nil {
			clients[i] = nil
		}
	}

	return clients
}

func GetConsulConfigFromDB() (*Configurations, error) {
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
