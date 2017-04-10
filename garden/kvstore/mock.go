package kvstore

import (
	"errors"
	"strings"

	"github.com/docker/swarm/garden/structs"
	"github.com/hashicorp/consul/api"
	"golang.org/x/net/context"
)

type mockClient struct {
	kv map[string][]byte
}

func NewMockClient() Client {
	return &mockClient{kv: make(map[string][]byte)}
}

func (c mockClient) GetHorusAddr() (string, error) {
	return "", errors.New("unsupport")
}

func (c mockClient) GetKV(key string) (*api.KVPair, error) {
	val, ok := c.kv[key]
	if ok {
		return &api.KVPair{
			Key:   key,
			Value: val,
		}, nil
	}

	return nil, errors.New("not found KV by:" + key)
}

func (c mockClient) ListKV(key string) (api.KVPairs, error) {
	out := make([]*api.KVPair, 0, 5)
	for k, val := range c.kv {
		if strings.HasPrefix(k, key) {
			out = append(out, &api.KVPair{
				Key:   key,
				Value: val,
			})
		}
	}

	return out, nil
}

func (c *mockClient) PutKV(key string, value []byte) error {
	c.kv[key] = value

	return nil
}

func (c *mockClient) DeleteKVTree(key string) error {
	for k := range c.kv {
		if strings.HasPrefix(k, key) {

			delete(c.kv, k)
		}
	}

	return nil
}

func (c mockClient) HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	return nil, nil
}

func (c *mockClient) RegisterService(ctx context.Context, host string, config structs.ServiceRegistration) error {
	return nil
}

func (c *mockClient) DeregisterService(ctx context.Context, config structs.ServiceDeregistration) error {
	return nil
}
