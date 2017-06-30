package kvstore

import (
	"encoding/json"
	stderr "errors"
	"fmt"
	"net"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var errUnavailableKVClient = stderr.New("non-available consul client")

func getStatus(c *api.Client, port string) (string, []string, error) {
	if c == nil {
		return "", nil, stderr.New("apiClient is required")
	}

	leader, err := c.Status().Leader()
	if err != nil {
		return "", nil, errors.WithStack(err)
	}

	host, _, err := net.SplitHostPort(leader)
	if err != nil {
		return "", nil, errors.Wrap(err, "split host port:"+leader)
	}
	leader = net.JoinHostPort(host, port)

	peers, err := c.Status().Peers()
	if err != nil {
		return "", nil, errors.WithStack(err)
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

// HealthChecksFromConsul is used to retrieve all the checks in a given state.
// The wildcard "any" state can also be used for all checks.
func (c *kvClient) HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	checks, _, err := client.Health().Checks(state, q)
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

	err = client.Agent().ServiceRegister(&config)
	c.checkConnectError(addr, err)
	if err == nil {
		return nil
	}

	return errors.WithStack(err)
}

func (c *kvClient) deregisterHealthCheck(host, ID string) error {
	addr, client, err := c.getClient(host)
	if err != nil {
		return err
	}

	err = client.Agent().ServiceDeregister(ID)
	c.checkConnectError(addr, err)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "deregister healthCheck")
}

// GetKV lookup a single key of KV store
func (c *kvClient) GetKV(ctx context.Context, key string) (*api.KVPair, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	key = c.key(key)

	var q *api.QueryOptions
	if ctx != nil {
		q = &api.QueryOptions{
			Context: ctx,
		}
	}

	val, _, err := client.KV().Get(key, q)
	c.checkConnectError(addr, err)
	if err == nil {
		return val, nil
	}

	return nil, errors.Wrap(err, "get KVPair:"+key)
}

func (c *kvClient) ListKV(ctx context.Context, key string) (api.KVPairs, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	key = c.key(key)

	var q *api.QueryOptions
	if ctx != nil {
		q = &api.QueryOptions{
			Context: ctx,
		}
	}

	val, _, err := client.KV().List(key, q)
	c.checkConnectError(addr, err)
	if err == nil {
		return val, nil
	}

	return nil, errors.Wrap(err, "list KVPairs:"+key)
}

func (c *kvClient) PutKV(ctx context.Context, key string, val []byte) error {
	addr, client, err := c.getClient("")
	if err != nil {
		return err
	}

	pair := &api.KVPair{
		Key:   c.key(key),
		Value: val,
	}
	_, err = client.KV().Put(pair, nil)
	c.checkConnectError(addr, err)
	if err == nil {
		return nil
	}

	return errors.WithStack(err)
}

func (c *kvClient) DeleteKVTree(ctx context.Context, key string) error {
	addr, client, err := c.getClient("")
	if err != nil {
		return err
	}

	key = c.key(key)

	_, err = client.KV().DeleteTree(key, nil)
	c.checkConnectError(addr, err)
	if err == nil {
		return nil
	}

	return errors.Wrap(err, "delete KV Tree:"+key)
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
