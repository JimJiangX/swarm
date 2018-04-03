package kvstore

import (
	stderr "errors"
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
func (c *kvClient) HealthChecks(ctx context.Context, state string) (map[string]api.HealthCheck, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	var q *api.QueryOptions
	if ctx != nil {
		q = q.WithContext(ctx)
	}

	checks, _, err := client.Health().Checks(state, q)
	c.checkConnectError(addr, err)
	if err != nil {
		return nil, errors.WithStack(err)
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

	return errors.WithStack(err)
}

func (c *kvClient) deregisterHealthCheck(host, ID string) error {
	addr, client, err := c.getClient(host)
	if err != nil {
		return err
	}

	err = client.Agent().ServiceDeregister(ID)
	c.checkConnectError(addr, err)

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
		q = q.WithContext(ctx)
	}

	val, _, err := client.KV().Get(key, q)
	c.checkConnectError(addr, err)

	return val, errors.Wrap(err, "get KVPair:"+key)
}

func (c *kvClient) ListKV(ctx context.Context, key string) (api.KVPairs, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	key = c.key(key)

	var q *api.QueryOptions
	if ctx != nil {
		q = q.WithContext(ctx)
	}

	val, _, err := client.KV().List(key, q)
	c.checkConnectError(addr, err)

	return val, errors.Wrap(err, "list KVPairs:"+key)
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

	var wo *api.WriteOptions
	if ctx != nil {
		wo = wo.WithContext(ctx)
	}

	_, err = client.KV().Put(pair, wo)
	c.checkConnectError(addr, err)

	return errors.WithStack(err)
}

func (c *kvClient) DeleteKVTree(ctx context.Context, key string) error {
	addr, client, err := c.getClient("")
	if err != nil {
		return err
	}

	var wo *api.WriteOptions
	if ctx != nil {
		wo = wo.WithContext(ctx)
	}

	key = c.key(key)

	_, err = client.KV().DeleteTree(key, wo)
	c.checkConnectError(addr, err)

	return errors.Wrap(err, "delete KV Tree:"+key)
}
