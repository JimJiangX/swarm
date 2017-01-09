package swarm

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
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

// HealthChecksFromConsul is used to retrieve all the checks in a given state.
// The wildcard "any" state can also be used for all checks.
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

// GetUnitRoleFromConsul lookup a single key of KV store
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
		return nil, errors.New("wrong KEY:" + key)
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
	var (
		err  error
		user database.User
	)

	if u.Type == _MysqlType {
		user, err = context.getUserByRole(_User_Check_Role)
		if err != nil {
			return err
		}
	}

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
		logrus.WithField("Unit", u.Name).WithError(err).Errorf("register healthCheck:%v", c)

		return errors.Wrap(err, "register unit healthCheck")
	}

	check, err := u.HealthCheck(u.Name, user.Username, user.Password)
	if err != nil {

		if err = u.factory(); err != nil {
			logrus.WithField("Unit", u.Name).WithError(err).Error("unit factory")
			return err
		}

		check, err = u.HealthCheck(u.Name, user.Username, user.Password)
		if err != nil {
			logrus.WithField("Unit", u.Name).WithError(err).Error("register healthCheck")

			return err
		}
	}

	if u.Type == _UpsqlType {
		swm, err := context.getSwithManagerUnit()
		if err == nil && swm != nil {
			check.Tags = []string{fmt.Sprintf("swm_key=%s/%s/Topology", context.ID, swm.Name)}
		}
	}

	containerID := u.ContainerID
	if u.container != nil && containerID != u.container.ID {
		containerID = u.container.ID
		u.ContainerID = u.container.ID
	}

	if check.Addr == "" {
		ips, err := u.getNetworkings()
		if err != nil {
			return err
		}
		for i := range ips {
			if ips[i].Type == _ContainersNetworking {
				check.Addr = ips[i].IP.String()
				break
			}
		}
	}

	service := api.AgentServiceRegistration{
		ID:      u.ID,
		Name:    u.Name,
		Tags:    check.Tags,
		Port:    check.Port,
		Address: check.Addr,
		Check: &api.AgentServiceCheck{
			Script:            check.Script,
			DockerContainerID: check.DockerContainerID,
			Shell:             check.Shell,
			Interval:          check.Interval,
			Timeout:           check.Timeout,
			TTL:               check.TTL,
			TCP:               check.TCP,
			HTTP:              check.HTTP,
			Status:            check.Status,
		},
	}

	logrus.Debugf("Agent Service Registration:%v %v", service, service.Check)

	err = client.Agent().ServiceRegister(&service)

	return errors.Wrap(err, "register unit service")
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
		return errors.Wrap(err, "deregister healthCheck")
	}

	err = client.Agent().ServiceDeregister(serviceID)

	return errors.Wrap(err, "deregister healthCheck")
}

func saveContainerToConsul(container *cluster.Container) error {
	client, err := getConsulClient(true)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(nil)
	err = json.NewEncoder(buf).Encode(container)
	if err != nil {
		return errors.Wrap(err, "encode container")
	}

	pair := &api.KVPair{
		Key:   _ContainerKVKeyPrefix + container.ID,
		Value: buf.Bytes(),
	}
	_, err = client.KV().Put(pair, nil)

	return errors.Wrap(err, "put KV")
}

func getContainerFromConsul(containerID string) (*cluster.Container, error) {
	client, err := getConsulClient(true)
	if err != nil {
		return nil, err
	}
	key := _ContainerKVKeyPrefix + containerID
	val, _, err := client.KV().Get(key, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "get KV from key:"+key)
	}

	c := cluster.Container{}
	err = json.Unmarshal(val.Value, &c)
	if err != nil {
		return nil, errors.Wrapf(err, "json decode error,value:\n%s", string(val.Value))
	}

	return &c, nil
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
		return "", errors.Wrap(err, "passing health checks")
	}

	for i := range checks {
		addr := parseIPFromHealthCheck(checks[i].ServiceID, checks[i].Output)
		if addr != "" {
			return addr, nil
		}
	}

	return "", errors.New("non-available Horus query from consul servers")
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

var errAvailableConsulClient = stderrors.New("non-available consul client")
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
		return errors.Errorf("unable to connect consul servers:%s", addrs)
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

	return nil, errors.Wrap(errAvailableConsulClient, "get consul client")
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
