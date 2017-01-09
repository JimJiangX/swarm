package kvstore

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

var errUnavailableKVClient = stderrors.New("non-available consul client")

type consulClient struct {
	client *api.Client
}

func newConsulClient(config *api.Config) (kvClientAPI, error) {
	client, err := api.NewClient(config)

	return consulClient{client}, err
}

func (client consulClient) getStatus(port string) (string, []string, error) {
	if client.client == nil {
		return "", nil, stderrors.New("consul API Client is required")
	}

	status := client.client.Status()

	leader, err := status.Leader()
	if err != nil {
		return "", nil, errors.Wrap(err, "get consul leader")
	}

	host, _, err := net.SplitHostPort(leader)
	if err != nil {
		return "", nil, errors.Wrap(err, "split host port:"+leader)
	}
	leader = net.JoinHostPort(host, port)

	peers, err := status.Peers()
	if err != nil {
		return "", nil, errors.Wrap(err, "get consul peers")
	}

	addrs := make([]string, 0, len(peers))
	for _, peer := range peers {
		host, _, err := net.SplitHostPort(peer)
		if err != nil {
			continue
		}

		addrs = append(addrs, net.JoinHostPort(host, port))
	}

	return leader, addrs, nil
}

func (c consulClient) getKV(key string, q *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error) {
	if c.client != nil {
		return c.client.KV().Get(key, q)
	}

	return nil, nil, stderrors.New("consul API Client is required")
}

func (c consulClient) putKV(p *api.KVPair, q *api.WriteOptions) (*api.WriteMeta, error) {
	if c.client != nil {
		return c.client.KV().Put(p, q)
	}

	return nil, stderrors.New("consul API Client is required")
}

func (c consulClient) deleteKVTree(prefix string, w *api.WriteOptions) (*api.WriteMeta, error) {
	if c.client != nil {
		return c.client.KV().DeleteTree(prefix, w)
	}

	return nil, stderrors.New("consul API Client is required")
}

func (c consulClient) healthChecks(service string, q *api.QueryOptions) ([]*api.HealthCheck, *api.QueryMeta, error) {
	if c.client != nil {
		return c.client.Health().Checks(service, q)
	}

	return nil, nil, stderrors.New("consul API Client is required")
}

func (c consulClient) serviceRegister(service *api.AgentServiceRegistration) error {
	if c.client != nil {
		return c.client.Agent().ServiceRegister(service)
	}

	return stderrors.New("consul API Client is required")
}

func (c consulClient) serviceDeregister(service string) error {
	if c.client != nil {
		return c.client.Agent().ServiceDeregister(service)
	}

	return stderrors.New("consul API Client is required")
}

// HealthChecksFromConsul is used to retrieve all the checks in a given state.
// The wildcard "any" state can also be used for all checks.
func (c *kvClient) HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	checks, _, err := client.healthChecks(state, q)
	c.checkConnectError(addr, err)
	if err != nil {
		return nil, err
	}

	m := make(map[string]api.HealthCheck, len(checks))
	for _, val := range checks {
		m[val.ServiceID] = *val
	}

	return m, nil
}

func (c *kvClient) registerHealthCheck(host string, config api.AgentServiceRegistration) error {
	addr, client, err := c.getClient(host)
	if err != nil {
		return err
	}

	err = client.serviceRegister(&config)
	c.checkConnectError(addr, err)

	return errors.Wrap(err, "register unit service")
}

func (c *kvClient) deregisterHealthCheck(host, ID string) error {
	addr, client, err := c.getClient(host)
	if err != nil {
		return err
	}

	err = client.serviceDeregister(ID)
	c.checkConnectError(addr, err)

	return errors.Wrap(err, "deregister healthCheck")
}

func (c *kvClient) PutKV(key string, val []byte) error {
	addr, client, err := c.getClient("")
	if err != nil {
		return err
	}

	pair := &api.KVPair{
		Key:   c.key(key),
		Value: val,
	}
	_, err = client.putKV(pair, nil)
	c.checkConnectError(addr, err)

	return errors.Wrap(err, "put KV")
}

func (c *kvClient) DeleteKVTree(key string) error {
	addr, client, err := c.getClient("")
	if err != nil {
		return err
	}

	key = c.key(key)

	_, err = client.deleteKVTree(key, nil)
	c.checkConnectError(addr, err)

	return errors.Wrap(err, "delete KV Tree:"+key)
}

// GetKV lookup a single key of KV store
func (c *kvClient) GetKV(key string) (*api.KVPair, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	key = c.key(key)

	val, _, err := client.getKV(key, nil)
	c.checkConnectError(addr, err)

	if err == nil {
		return val, nil
	}

	return nil, errors.Wrap(err, "get KVPair:"+key)
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
		return nil, err
	}

	m := make(map[string]string, len(roles.Units.Default))
	for key, val := range roles.Units.Default {
		m[key] = fmt.Sprintf("%s(%s)", val.Type, val.Status)
	}

	return m, nil
}
