package kvstore

import "github.com/hashicorp/consul/api"

type kvClientAPI interface {
	getStatus(port string) (string, []string, error)

	getKV(key string, q *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error)
	putKV(p *api.KVPair, q *api.WriteOptions) (*api.WriteMeta, error)
	deleteKVTree(prefix string, w *api.WriteOptions) (*api.WriteMeta, error)

	healthChecks(service string, q *api.QueryOptions) ([]*api.HealthCheck, *api.QueryMeta, error)
	serviceRegister(service *api.AgentServiceRegistration) error
	serviceDeregister(service string) error
}

type Client interface {
	GetHorusAddr() (string, error)
}
