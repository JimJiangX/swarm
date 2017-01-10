package kvstore

import (
	"context"

	"github.com/docker/swarm/garden/structs"
	"github.com/hashicorp/consul/api"
)

type kvClientAPI interface {
	getStatus(port string) (string, []string, error)

	getKV(key string, q *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error)
	listKV(prefix string, q *api.QueryOptions) (api.KVPairs, *api.QueryMeta, error)
	putKV(p *api.KVPair, q *api.WriteOptions) (*api.WriteMeta, error)
	deleteKVTree(prefix string, w *api.WriteOptions) (*api.WriteMeta, error)

	healthChecks(service string, q *api.QueryOptions) ([]*api.HealthCheck, *api.QueryMeta, error)
	serviceRegister(service *api.AgentServiceRegistration) error
	serviceDeregister(service string) error
}

type Client interface {
	Register

	GetHorusAddr() (string, error)

	GetKV(key string) (*api.KVPair, error)
	ListKV(key string) (api.KVPairs, error)
	PutKV(key string, val []byte) error
	DeleteKVTree(key string) error
}

type Register interface {
	HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error)

	RegisterService(ctx context.Context, host string, config api.AgentServiceRegistration, obj structs.HorusRegistration) error

	DeregisterService(ctx context.Context, addr, key string) error
}
