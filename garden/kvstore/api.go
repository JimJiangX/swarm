package kvstore

import (
	"github.com/docker/swarm/garden/structs"
	"github.com/hashicorp/consul/api"
	"golang.org/x/net/context"
)

// Client is a kv store interface
type Client interface {
	Register

	GetHorusAddr() (string, error)

	GetKV(key string) (*api.KVPair, error)
	ListKV(key string) (api.KVPairs, error)
	PutKV(key string, value []byte) error
	DeleteKVTree(key string) error
}

const (
	hostType      = "hosts"
	unitType      = "units"
	containerType = "containers"
)

// Register is a client for register service
type Register interface {
	HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error)

	RegisterService(ctx context.Context, host string, config structs.ServiceRegistration) error

	DeregisterService(ctx context.Context, config structs.ServiceDeregistration, force bool) error
}
