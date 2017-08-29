package parser

import (
	"net/http"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	pclient "github.com/docker/swarm/plugin/parser/api"
	"golang.org/x/net/context"
)

var pc pclient.PluginAPI

func init() {
	kvc := kvstore.NewMockClient()
	mux := NewRouter(kvc, ".", "", 0)

	go http.ListenAndServe(":8080", mux)

	pc = pclient.NewPlugin("localhost:8080", 30*time.Second, nil)
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
		Image:     "redis:3.2.8.0",
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
		Image: "redis:3.2.8.0",
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
			Networking: []structs.UnitIP{
				{
					IP: "192.168.4.141",
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
			Networking: []structs.UnitIP{
				{
					IP: "192.168.4.142",
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
			Networking: []structs.UnitIP{
				{
					IP: "192.168.4.143",
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
			Image: "redis:3.2.8.0",
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
		t.Errorf("got configs %d", len(configs))
	}
}

func TestGetConfigs(t *testing.T) {
	cm, err := pc.GetServiceConfig(context.Background(), "serivce0001")
	if err != nil {
		t.Error(err)
	}

	for id, val := range cm {
		t.Log(id, val.ID, val.GetServiceRegistration().Horus == nil)
	}
}

func TestGetConfig(t *testing.T) {
	cc, err := pc.GetUnitConfig(context.Background(), "serivce0001", "unitXXX002")
	if err != nil {
		t.Error(err)
	}

	t.Log(cc.GetCmd(structs.InitServiceCmd), cc.GetServiceRegistration().Horus == nil)
}
