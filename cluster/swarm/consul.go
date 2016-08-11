package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

func HealthChecksFromConsul(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	client, err := getConsulClient(true)
	if err != nil {
		return nil, err
	}

	checks, _, err := client.Health().State(state, q)
	if err != nil {
		return nil, err
	}

	m := make(map[string]api.HealthCheck, len(checks))
	for _, val := range checks {
		m[val.ServiceID] = *val
	}

	return m, nil
}

func GetUnitRoleFromConsul(key string) (map[string]string, error) {
	client, err := getConsulClient(true)
	if err != nil {
		return nil, err
	}

	key = key + "/Topology"
	val, _, err := client.KV().Get(key, nil)
	if err != nil {
		logrus.Error(err, key)
		return nil, err
	}

	if val == nil {
		return nil, fmt.Errorf("Wrong KEY:%s", key)
	}

	return rolesJSONUnmarshal(val.Value)
}

func rolesJSONUnmarshal(data []byte) (map[string]string, error) {
	roles := struct {
		Units struct {
			Default map[string]struct {
				Type   string
				Status string
			}
		} `json:"datanode_group"`
	}{}

	err := json.Unmarshal(data, &roles)
	if err != nil {
		logrus.Error(err, string(data))
		return nil, err
	}

	m := make(map[string]string, len(roles.Units.Default))
	for key, val := range roles.Units.Default {
		m[key] = fmt.Sprintf("%s(%s)", val.Type, val.Status)
	}

	return m, nil
}

func registerHealthCheck(u *unit, context *Service) error {
	eng, err := u.Engine()
	if err != nil {
		return err
	}

	config, err := getConsulConfig()
	if err != nil {
		return err
	}

	address := fmt.Sprintf("%s:%d", eng.IP, config.ConsulPort)

	c := api.Config{
		Address:    address,
		Datacenter: config.ConsulDatacenter,
		WaitTime:   time.Duration(config.ConsulWaitTime) * time.Second,
		Token:      config.ConsulToken,
	}
	client, err := api.NewClient(&c)
	if err != nil {
		logrus.Errorf("%s Register HealthCheck Error,%s %v", u.Name, err, c)
		return err
	}

	check, err := u.HealthCheck()
	if err != nil {
		logrus.Warnf("Unit %s HealthCheck error:%s", u.Name, err)
		if err = u.factory(); err != nil {
			logrus.Error("Unit %s factory error:%s", u.Name, err)
			return err
		}

		check, err = u.HealthCheck()
		if err != nil {
			logrus.Errorf("Unit %s HealthCheck error:%s", u.Name, err)

			return err
		}
	}

	if u.Type == _UpsqlType {
		swm := context.getSwithManagerUnit()
		if swm != nil {
			check.Tags = []string{fmt.Sprintf("swm_key=%s/%s/Topology", context.ID, swm.Name)}
		}
	}

	containerID := u.ContainerID
	if u.container != nil && containerID != u.container.ID {
		containerID = u.container.ID
		u.ContainerID = u.container.ID
	}

	addr := ""
	ips, err := u.getNetworkings()
	if err != nil {
		return err
	}
	for i := range ips {
		if ips[i].Type == _ContainersNetworking {
			addr = ips[i].IP.String()
		}
	}

	service := api.AgentServiceRegistration{
		ID:      u.ID,
		Name:    u.Name,
		Tags:    check.Tags,
		Port:    check.Port,
		Address: addr,
		Check: &api.AgentServiceCheck{
			Script: check.Script + u.Name,
			// DockerContainerID: containerID,
			Shell:    check.Shell,
			Interval: check.Interval,
			// TTL:      check.TTL,
		},
	}

	logrus.Debugf("AgentServiceRegistration:%v %v", service, service.Check)

	return client.Agent().ServiceRegister(&service)
}

func deregisterHealthCheck(host, serviceID string) error {
	config, err := getConsulConfig()
	if err != nil {
		return err
	}

	address := fmt.Sprintf("%s:%d", host, config.ConsulPort)

	c := api.Config{
		Address:    address,
		Datacenter: config.ConsulDatacenter,
		WaitTime:   time.Duration(config.ConsulWaitTime) * time.Second,
		Token:      config.ConsulToken,
	}
	client, err := api.NewClient(&c)
	if err != nil {
		return err
	}

	return client.Agent().ServiceDeregister(serviceID)
}

func saveContainerToConsul(container *cluster.Container) error {
	client, err := getConsulClient(true)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(nil)
	err = json.NewEncoder(buf).Encode(container)
	if err != nil {
		return err
	}

	pair := &api.KVPair{
		Key:   "DBAAS/Conatainers/" + container.ID,
		Value: buf.Bytes(),
	}
	_, err = client.KV().Put(pair, nil)

	return err
}

func deleteConsulKVTree(key string) error {
	client, err := getConsulClient(true)
	if err != nil {
		return err
	}

	_, err = client.KV().DeleteTree(key, nil)

	return err
}

func getHorusFromConsul() (string, error) {
	client, err := getConsulClient(true)
	if err != nil {
		return "", err
	}

	checks, _, err := client.Health().State("passing", nil)
	if err != nil {
		return "", err
	}

	for i := range checks {
		addr := parseIPFromHealthCheck(checks[i].ServiceID, checks[i].Output)
		if addr != "" {
			return addr, nil
		}
	}

	return "", errors.New("Non Available Horus Query From Consul Servers")
}

func parseIPFromHealthCheck(serviceID, output string) string {
	const key = "HS-"

	if !strings.HasPrefix(serviceID, key) {
		return ""
	}

	index := strings.Index(serviceID, key)
	addr := string(serviceID[index+len(key):])

	if net.ParseIP(addr) == nil {
		return ""
	}

	index = strings.Index(output, addr)

	parts := strings.Split(string(output[index:]), ":")
	if len(parts) >= 2 {

		addr = parts[0] + ":" + parts[1]
		_, _, err := net.SplitHostPort(addr)
		if err == nil {
			return addr
		}
	}

	return ""
}

var errAvailableConsulClient = errors.New("Non Available Consul Client")
var defaultConsuls = &consulConfigs{}

func getConsulClient(ping bool) (*api.Client, error) {
	client, err := defaultConsuls.getConsulClient(ping)
	if err == nil {
		return client, nil
	}

	c, err := database.GetSystemConfig()
	if err == nil {
		err = setConsulClient(&c.ConsulConfig)
		if err == nil {
			return defaultConsuls.getConsulClient(ping)
		}
	}

	return nil, err
}

func getConsulConfig() (database.ConsulConfig, error) {

	return defaultConsuls.getConsulConfig()
}

func setConsulClient(c *database.ConsulConfig) error {
	if c == nil {
		c = &defaultConsuls.c
	}

	return defaultConsuls.set(*c)
}

type consulConfigs struct {
	sync.RWMutex
	clients []*api.Client
	addrs   []string
	c       database.ConsulConfig
}

func (cs *consulConfigs) set(c database.ConsulConfig) error {
	cs.Lock()
	defer cs.Unlock()

	addrs := strings.Split(c.ConsulIPs, ",")
	addrs = append(addrs, "127.0.0.1")

	config := api.Config{
		Datacenter: c.ConsulDatacenter,
		WaitTime:   time.Duration(c.ConsulWaitTime) * time.Second,
		Token:      c.ConsulToken,
	}

	var (
		peers []string
		port  = strconv.Itoa(c.ConsulPort)
	)
	for i := range addrs {
		config.Address = addrs[i] + ":" + port

		client, err := api.NewClient(&config)
		if err != nil {
			continue
		}

		peers, err = client.Status().Peers()
		if err == nil && len(peers) > 0 {
			break
		}
	}

	if len(peers) == 0 {
		return errors.Errorf("Unable to connect consul servers,%s", addrs)
	}

	list := make([]*api.Client, len(peers))

	for i := range peers {
		host, _, err := net.SplitHostPort(peers[i])
		if err != nil {
			continue
		}

		config.Address = host + ":" + port

		client, err := api.NewClient(&config)
		if err != nil {
			continue
		}

		list[i] = client
	}

	cs.addrs = addrs
	cs.c = c
	cs.clients = list

	return nil
}

func (cs *consulConfigs) getConsulClient(ping bool) (*api.Client, error) {
	var client *api.Client

	cs.RLock()

	for _, c := range cs.clients {
		if c == nil {
			continue
		}

		if ping {
			_, err := c.Status().Peers()
			if err != nil {
				continue
			}
		}

		client = c
		break
	}

	cs.RUnlock()

	if client != nil {
		return client, nil
	}

	return nil, errAvailableConsulClient
}

func (cs *consulConfigs) getConsulConfig() (database.ConsulConfig, error) {
	cs.RLock()

	config := cs.c
	cs.RUnlock()

	if config.ConsulPort == 0 {
		return config, errors.New("Non ConsulConfig")
	}

	return config, nil
}
