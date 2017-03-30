package parser

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/plugin/client"
	pclient "github.com/docker/swarm/plugin/parser/api"
	"github.com/hashicorp/consul/api"
	"golang.org/x/net/context"
)

var pc pclient.PluginAPI = nil

func init() {
	kvc := kvclient{make(map[string][]byte)}
	mux := NewRouter(kvc, "", 0)

	go http.ListenAndServe(":8080", mux)

	cli := client.NewClient("localhost:8080", 30*time.Second, nil)
	pc = pclient.NewPlugin(cli)
}

type kvclient struct {
	kv map[string][]byte
}

var _ kvstore.Client = kvclient{}

func (c kvclient) GetHorusAddr() (string, error) {
	return "", errors.New("unsupport")
}

func (c kvclient) GetKV(key string) (*api.KVPair, error) {
	val, ok := c.kv[key]
	if ok {
		return &api.KVPair{
			Key:   key,
			Value: val,
		}, nil
	}

	return nil, errors.New("not found KV by:" + key)
}

func (c kvclient) ListKV(key string) (api.KVPairs, error) {
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

func (c kvclient) PutKV(key string, value []byte) error {
	c.kv[key] = value

	return nil
}

func (c kvclient) DeleteKVTree(key string) error {
	for k := range c.kv {
		if strings.HasPrefix(k, key) {

			delete(c.kv, k)
		}
	}

	return nil
}

func (c kvclient) HealthChecks(state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	return nil, nil
}

func (c kvclient) RegisterService(ctx context.Context, host string, config structs.ServiceRegistration) error {
	return nil
}

func (c kvclient) DeregisterService(ctx context.Context, typ, key, user, pwd string) error {
	return nil
}

func TestGetSupportImageVersion(t *testing.T) {
	out, err := pc.GetImageSupport(context.Background())
	if err != nil {
		t.Error(err)
	}

	if len(out) != len(images) {
		t.Error("images:", len(out))
	}
}

func TestPostTemplate(t *testing.T) {
	ct := structs.ConfigTemplate{
		Image:     "redis:3.2.8",
		LogMount:  "/usr/local/log",
		DataMount: "/usr/local/data",
		Content:   configTemplateContext,
	}

	err := pc.PostImageTemplate(context.Background(), ct)
	if err != nil {
		t.Error(err)
	}
}

const configTemplateContext = `
bind 192.168.4.141
port 6379
dir /UPM/DAT
dbfilename redis.rdb
appendfilename appendonly.aof
pidfile /UPM/DAT/redis.pid
logfile /UPM/DAT/redis.log
maxmemory 300M
cluster-enabled yes
cluster-config-file redis_6379.conf
cluster-node-timeout 5000
appendonly yes
tcp-backlog 511
timeout 60
tcp-keepalive 0
loglevel notice
databases 16
save 900 1
save 300 10
save 60 10000
stop-writes-on-bgsave-error yes
rdbcompression yes
rdbchecksum yes
slave-serve-stale-data yes
slave-read-only yes
repl-diskless-sync no
repl-diskless-sync-delay 5
repl-disable-tcp-nodelay no
slave-priority 100
maxmemory-policy noeviction
appendonly yes
appendfsync everysec
no-appendfsync-on-rewrite yes
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb
aof-load-truncated yes
lua-time-limit 5000
slowlog-log-slower-than 10000
slowlog-max-len 1000
latency-monitor-threshold 0
notify-keyspace-events ""
hash-max-ziplist-entries 512
hash-max-ziplist-value 64
list-max-ziplist-entries 512
list-max-ziplist-value 64
set-max-intset-entries 512
zset-max-ziplist-entries 128
zset-max-ziplist-value 64
hll-sparse-max-bytes 3000
activerehashing yes
client-output-buffer-limit normal 0 0 0
client-output-buffer-limit slave 256mb 64mb 60
client-output-buffer-limit pubsub 32mb 8mb 60
hz 10
aof-rewrite-incremental-fsync yes
daemonize yes
`

var redisSpec = structs.ServiceSpec{
	Service: structs.Service{
		ID:    "serivce0001",
		Name:  "redis_service_001",
		Image: "redis:3.2.8",
	},
	Arch: structs.Arch{
		Mode:     "sharding_replication",
		Replicas: 3,
		Code:     "m:3#s:0",
	},
	Options: map[string]interface{}{
		"port": 8327,
	},
	Units: []structs.UnitSpec{
		{
			Unit: structs.Unit{
				ID: "unitXXX001",
			},
			Config: &cluster.ContainerConfig{
				HostConfig: container.HostConfig{
					Resources: container.Resources{
						CpusetCpus: "0,1",
						Memory:     1 << 30,
					},
				},
			},
			Networking: structs.UnitNetworking{
				IPs: []structs.UnitIP{
					{
						IP: "192.168.4.141",
					},
				},
			},
		},

		{
			Unit: structs.Unit{
				ID: "unitXXX002",
			},
			Config: &cluster.ContainerConfig{
				HostConfig: container.HostConfig{
					Resources: container.Resources{
						CpusetCpus: "0,1",
						Memory:     1 << 31,
					},
				},
			},
			Networking: structs.UnitNetworking{
				IPs: []structs.UnitIP{
					{
						IP: "192.168.4.142",
					},
				},
			},
		},

		{
			Unit: structs.Unit{
				ID: "unitXXX003",
			},
			Config: &cluster.ContainerConfig{
				HostConfig: container.HostConfig{
					Resources: container.Resources{
						CpusetCpus: "0,1",
						Memory:     1 << 32,
					},
				},
			},
			Networking: structs.UnitNetworking{
				IPs: []structs.UnitIP{
					{
						IP: "192.168.4.143",
					},
				},
			},
		},
	},
}

func TestGenerateConfigs(t *testing.T) {
	configs, err := pc.GenerateServiceConfig(context.Background(), structs.ServiceSpec{})
	if err == nil {
		t.Error("expect error")
	}

	configs, err = pc.GenerateServiceConfig(context.Background(), structs.ServiceSpec{
		Service: structs.Service{
			ID:    "serivce0001",
			Name:  "redis_service_001",
			Image: "redis:3.2.8",
		},
		Options: map[string]interface{}{
			"port": 8327,
		},
	})
	if err != nil || configs == nil {
		t.Error(err)
	}

	configs, err = pc.GenerateServiceConfig(context.Background(), redisSpec)
	if err != nil {
		t.Error(err)
	}

	if len(configs) != len(redisSpec.Units) {
		t.Error("got configs %d", len(configs))
	}
}
