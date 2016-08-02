package swarm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/hashicorp/consul/api"
)

var ErrConsulClientIsNil = errors.New("consul client is nil")

func HealthChecksFromConsul(client *api.Client, state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	if client == nil {
		return nil, ErrConsulClientIsNil
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

func GetUnitRoleFromConsul(client *api.Client, key string) (map[string]string, error) {
	if client == nil {
		return nil, ErrConsulClientIsNil
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

func (gd *Gardener) setConsulClient(client *api.Client) {
	gd.Lock()
	gd.consulClient = client
	gd.Unlock()
}

func (gd *Gardener) ConsulAPIClient() (*api.Client, error) {
	gd.RLock()
	if gd.consulClient != nil {
		if _, err := gd.consulClient.Status().Leader(); err == nil {
			gd.RUnlock()
			return gd.consulClient, nil
		}
	}
	gd.RUnlock()

	sys, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	_, clients := pingConsul(HostAddress, *sys)
	if len(clients) > 0 {
		return clients[0], nil
	}

	return nil, fmt.Errorf("Not Found Alive Consul Server %s:%d", sys.ConsulIPs, sys.ConsulPort)
}

func pingConsul(host string, sys database.Configurations) ([]string, []*api.Client) {
	endpoints, dc, token, wait := sys.GetConsulConfig()
	port := strconv.Itoa(sys.ConsulPort)
	endpoints = append(endpoints, host+":"+port)

	endpoints[0], endpoints[len(endpoints)-1] = endpoints[len(endpoints)-1], endpoints[0]

	peers := make([]string, 0, len(endpoints))
	clients := make([]*api.Client, 0, len(endpoints))

	for _, endpoint := range endpoints {
		config := api.Config{
			Address:    endpoint,
			Datacenter: dc,
			WaitTime:   time.Duration(wait) * time.Second,
			Token:      token,
		}

		client, err := api.NewClient(&config)
		if err != nil {
			logrus.Warnf("consul config illegal，%v", config)
			continue
		}

		servers, err := client.Status().Peers()
		if err != nil {
			logrus.Warnf("consul connection error,%s,%v", err, config)
			continue
		}

		addrs := make([]string, 0, len(servers)+1)
		for n := range servers {
			ip, _, err := net.SplitHostPort(servers[n])
			if err != nil {
				logrus.Warn("%s SplitHostPort error %s", servers[n], err)
				continue
			}

			servers[n] = ip + ":" + port
			addrs = append(addrs, servers[n])
		}

		exist := false
		for n := range servers {
			if endpoint == servers[n] {
				exist = true
				break
			}
		}
		if !exist {
			peers = append(peers, endpoint)
			peers = append(peers, servers...)

			clients = append(clients, client)
		} else {
			peers = servers
		}

		for i := range servers {
			config := api.Config{
				Address:    servers[i],
				Datacenter: dc,
				WaitTime:   time.Duration(wait) * time.Second,
				Token:      token,
			}

			client, err := api.NewClient(&config)
			if err != nil {
				logrus.Warnf("consul config illegal，%v", config)
				continue
			}
			clients = append(clients, client)
		}

		break
	}

	return peers, clients
}

func registerHealthCheck(u *unit, config database.ConsulConfig, context *Service) error {
	eng, err := u.Engine()
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
			check.Tags = []string{fmt.Sprintf("swm_key=%s/%s/topology", context.ID, swm.ID)}
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

func deregisterHealthCheck(host, serviceID string, config api.Config) error {
	_, port, err := net.SplitHostPort(config.Address)
	if err != nil {
		logrus.Error("SplitHostPort %s error %s", config.Address, err)
		return err
	}

	config.Address = fmt.Sprintf("%s:%s", host, port)
	client, err := api.NewClient(&config)
	if err != nil {
		return err
	}

	return client.Agent().ServiceDeregister(serviceID)
}

func deleteConsulKV(config api.Config, key string) error {
	client, err := api.NewClient(&config)
	if err != nil {
		return err
	}

	_, err = client.KV().Delete(key, nil)

	return err
}
