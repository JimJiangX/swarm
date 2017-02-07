package kvstore

import (
	"context"

	"github.com/docker/swarm/garden/structs"
	"github.com/hashicorp/consul/api"
)

type Client interface {
	Register

	GetHorusAddr() (string, error)

	GetKV(key string) (*api.KVPair, error)
	ListKV(key string) (api.KVPairs, error)
	PutKV(key string, value []byte) error
	DeleteKVTree(key string) error
}

type Register interface {
	HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error)

	RegisterService(ctx context.Context, host string, config structs.ServiceRegistration) error

	DeregisterService(ctx context.Context, addr, key string) error
}
