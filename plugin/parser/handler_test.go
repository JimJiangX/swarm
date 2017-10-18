package parser

import (
	"net/http"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/plugin/client"
	pclient "github.com/docker/swarm/plugin/parser/api"
	"golang.org/x/net/context"
)

var pc pclient.PluginAPI

func init() {
	kvc := kvstore.NewMockClient()
	mux := NewRouter(kvc, "", ".", "", 0)

	go http.ListenAndServe(":8080", mux)

	pc = pclient.NewPlugin("localhost:8080", client.NewClient("localhost:8080", 30*time.Second, nil))
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
		Content:   redisTemplateContext,
	}

	err := pc.PostImageTemplate(context.Background(), ct)
	if err != nil {
		t.Error(err)
	}

	ct = structs.ConfigTemplate{
		Image:     "upsql:2.0.2.1",
		LogMount:  "/usr/local/log",
		DataMount: "/usr/local/data",
		Content:   mysqlTemplateContext,
	}

	err = pc.PostImageTemplate(context.Background(), ct)
	if err != nil {
		t.Error(err)
	}
}

const redisTemplateContext = `
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

const mysqlTemplateContext = `

[mysqld]
bind_address =  <ip_addr>
port = <port>
socket = <data_dir>/upsql.sock
server_id = <port>
character_set_server = <char_set>
max_connect_errors = 50000
max_connections = 5000
max_user_connections = 0
skip_external_locking = ON
max_allowed_packet = 16M
sort_buffer_size = 2M
join_buffer_size = 128K
user = upsql
tmpdir = <data_dir>
datadir = <data_dir>
log_error = upsql.err
log_bin = <log_dir>/BIN/<container_name>-binlog
log_bin_trust_function_creators = ON
sync_binlog = 1
expire_logs_days = 0
key_buffer_size = 160M
binlog_cache_size = 1M
binlog_format = row
lower_case_table_names = 1
max_binlog_size = 1G
connect_timeout = 60
interactive_timeout = 31536000
wait_timeout = 31536000
net_read_timeout = 30
net_write_timeout = 60
optimizer_switch = 'mrr=on,mrr_cost_based=off'
open_files_limit = 10240
explicit_defaults_for_timestamp = true
slow_query_log=0
slow_query_log_file=<log_dir>/slow-query.log
long_query_time=1
log_queries_not_using_indexes=0
innodb_open_files = 1024
innodb_data_file_path=ibdata1:12M:autoextend
innodb_buffer_pool_size = <container_mem> * 0.75
innodb_buffer_pool_instances = 8
innodb_log_buffer_size = 128M
innodb_log_file_size = 128M
innodb_log_files_in_group = 7
innodb_log_group_home_dir = <log_dir>/RED
innodb_max_dirty_pages_pct = 30
innodb_flush_method = O_DIRECT
innodb_flush_log_at_trx_commit = 1
innodb_thread_concurrency = 16
innodb_read_io_threads = 4
innodb_write_io_threads = 4
innodb_lock_wait_timeout = 60
innodb_rollback_on_timeout = on
innodb_file_per_table = 1
innodb_stats_sample_pages = 1
innodb_purge_threads = 1
innodb_stats_on_metadata = OFF
innodb_support_xa = 1
innodb_doublewrite = 1
innodb_checksums = 1
innodb_io_capacity = 500
innodb_purge_threads = 8
innodb_purge_batch_size = 500
innodb_stats_persistent_sample_pages = 10
plugin_dir = /usr/local/mysql/lib/plugin
plugin_load = "rpl_semi_sync_master=semisync_master.so;rpl_semi_sync_slave=semisync_slave.so;upsql_auth=upsql_auth.so"
loose_rpl_semi_sync_master_enabled = 1
loose_rpl_semi_sync_slave_enabled = 1
gtid_mode = on
enforce_gtid_consistency = on
log_slave_updates = on
binlog_checksum = CRC32
binlog_row_image = minimal
slave_sql_verify_checksum = on
slave_parallel_type = LOGICAL_CLOCK
slave_parallel_workers = 128
master_verify_checksum  =   ON
slave_sql_verify_checksum = ON
master_info_repository=TABLE
relay_log_info_repository=TABLE
replicate_ignore_db=dbaas_check
rpl_semi_sync_master_enabled = on
auto_increment_increment = 1
auto_increment_offset = 1
rpl_semi_sync_master_timeout = 1000
rpl_semi_sync_master_wait_no_slave = on
rpl_semi_sync_master_trace_level = 32
rpl_semi_sync_slave_enabled = on
rpl_semi_sync_slave_trace_level = 32
rpl_stop_slave_timeout = 6000
slave_net_timeout = 10
relay_log_recovery = on
log_slave_updates = on
max_relay_log_size = 1G
relay_log = <log_dir>/REL/<container_name>-relay
relay_log_purge = on
[mysqldump]
max_allowed_packet = 16M
[myisamchk]
key_buffer_size = 20M
sort_buffer_size = 2M
[client]
socket = <data_dir>/upsql.sock 
user = <username>
password = <password>
host = localhost
`

var redisSpec = structs.ServiceSpec{
	Service: structs.Service{
		ID:    "service0001",
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
			ID:    "service0001",
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
	cm, err := pc.GetServiceConfig(context.Background(), "service0001")
	if err != nil {
		t.Error(err)
	}

	for id, val := range cm {
		t.Log(id, val.ID)
	}
}

func TestGetConfig(t *testing.T) {
	cc, err := pc.GetUnitConfig(context.Background(), "service0001", "unitXXX002")
	if err != nil {
		t.Errorf("%+v", err)
	}

	t.Log(cc.GetCmd(structs.InitServiceCmd), cc.GetServiceRegistration().Horus == nil)
}

func TestSetLeaderElectionPath(t *testing.T) {
	setLeaderElectionPath("consul://146.32.99.22:8400/unionpay/docker/swarm/leader")
	if consulPort != "8400" {
		t.Error(consulPort)
	}

	if leaderElectionPath != "/unionpay/docker/swarm/leader" {
		t.Error(leaderElectionPath)
	}

	if consulPrefix != "/unionpay" {
		t.Error(consulPrefix)
	}

	setLeaderElectionPath("146.32.99.22:8300/unionpay/docker/swarm/")
	if consulPort != "8300" {
		t.Error(consulPort)
	}

	if leaderElectionPath != "/unionpay/docker/swarm" {
		t.Error(leaderElectionPath)
	}
}
