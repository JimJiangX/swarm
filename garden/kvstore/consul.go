package kvstore

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

const _ContainerKVKeyPrefix = "container"

// HealthChecksFromConsul is used to retrieve all the checks in a given state.
// The wildcard "any" state can also be used for all checks.
func (c *kvClient) HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	checks, _, err := client.Health().State(state, q)
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

	return errors.Wrap(err, "register unit service")
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

func (c *kvClient) PutKV(key string, val []byte) error {
	pair := &api.KVPair{
		Key:   key,
		Value: val,
	}

	addr, client, err := c.getClient("")
	if err != nil {
		return err
	}

	_, err = client.KV().Put(pair, nil)
	c.checkConnectError(addr, err)

	return errors.Wrap(err, "put KV")
}

func (c *kvClient) DeleteKVTree(key string) error {
	addr, client, err := c.getClient("")
	if err != nil {
		return err
	}

	_, err = client.KV().DeleteTree(key, nil)
	c.checkConnectError(addr, err)

	return errors.Wrap(err, "delete KV Tree:"+key)
}

// GetKV lookup a single key of KV store
func (c *kvClient) GetKV(key string) (*api.KVPair, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return nil, err
	}

	val, _, err := client.KV().Get(key, nil)
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
