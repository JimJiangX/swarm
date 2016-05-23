package swarm

import (
	"fmt"
	"testing"
)

var defaultMysqlContent = `
################
##UpSQL 5.6.19##
################
[mysqld]
bind-address =  <ip_addr>
port = <port>
socket = /DBAASDAT/upsql.sock
server-id = <port>
character_set_server = <char_set>
max_connect_errors = 50000
max_connections = 5000
max_user_connections = 0
##skip-name-resolve
skip_external_locking = ON
max_allowed_packet = 16M
sort_buffer_size = 2M
join_buffer_size = 128K
user = upsql
tmpdir = /DBAASDAT
datadir = /DBAASDAT
log-bin = /DBAASLOG/BIN/<container_name>-binlog
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
innodb_open_files = 1024
innodb_data_file_path=ibdata1:12M:autoextend
innodb_buffer_pool_size = <container_mem> * 0.75
innodb_buffer_pool_instances = 8
innodb_log_buffer_size = 128M
innodb_log_file_size = 512M
innodb_log_files_in_group = 7
innodb_log_group_home_dir = /DBAASLOG/RED
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
##[DBPM variables]
#upsql_auth_dbpm_mainip=144.7.32.31
#upsql_auth_dbpm_bkupip=144.7.34.31
#upsql_auth_dbpm_mainport=20010
#upsql_auth_dbpm_bkupport=20010
#upsql_auth_update_timeslice=3600
#upsql_auth_dbpm_serverid=upsql
#upsql_auth_dbpm_tmtime=2
#upsql_ee_cheat_iplist=

##[Replication variables]
gtid-mode = on
enforce-gtid-consistency = on
log-slave-updates = on
binlog_checksum = CRC32
binlog_row_image = minimal
slave_sql_verify_checksum = on
slave_parallel_workers = 5
master_verify_checksum  =   ON
slave_sql_verify_checksum = ON
master_info_repository=TABLE
relay_log_info_repository=TABLE
replicate-ignore-db=dbaas_check

##[Replication variables for Master]
rpl_semi_sync_master_enabled = on
auto_increment_increment = 1
auto_increment_offset = 1
rpl_semi_sync_master_timeout = 10000
rpl_semi_sync_master_wait_no_slave = on
rpl_semi_sync_master_trace_level = 32

##[Replication variables for Slave]
rpl_semi_sync_slave_enabled = on
rpl_semi_sync_slave_trace_level = 32
slave_net_timeout = 10
relay_log_recovery = on
log_slave_updates = on
max_relay_log_size = 1G
relay_log = /DBAASLOG/REL/<container_name>-relay
relay_log_purge = on


[mysqldump]
##quick
max_allowed_packet = 16M

[myisamchk]
key_buffer_size = 20M
sort_buffer_size = 2M
read_buffer = 2M
write_buffer = 2M

[mysqlhotcopy]
##interactive-timeout
`

func TestMysqlConfig(t *testing.T) {
	config := new(mysqlConfig)

	data, err := config.defaultUserConfig(nil, nil)
	if err != nil {
		t.Logf("Expected Error:%s", err.Error())

		data = make(map[string]interface{}, 10)
		data["mysqld::character_set_server"] = "gbk"
		data["mysqld::log-bin"] = fmt.Sprintf("/DBAASLOG/BIN/%s-binlog", "test_unit")
		data["mysqld::innodb_buffer_pool_size"] = int(float64(1<<30) * 0.75)
		data["mysqld::relay_log"] = fmt.Sprintf("/DBAASLOG/REL/%s-relay", "test_unit")
		data["mysqld::port"] = 3306
		data["mysqld::server-id"] = 3306
		data["mysqld::bind-address"] = "127.0.0.1"
	}

	t.Log("Default Config:\n", defaultMysqlContent)

	_, err = config.ParseData([]byte(defaultMysqlContent))
	if err != nil {
		t.Fatal(err)
	}

	err = config.Validate(data)
	if err != nil {
		t.Error(err)
	}

	for key, val := range data {
		if err := config.Set(key, val); err != nil {
			t.Error(err)
		}
	}

	content, err := config.Marshal()
	if err != nil {
		t.Error(err)
	}

	t.Log("config ini:\n", string(content))
}

var defaultSWMConfig = `
#sm --defaults-file=/tmp/sm.conf
#required
### sm
Domain =  <service_id>
Name = <unit_id>
Port = <adm_port>
ProxyPort = <proxy_port>
#optional(default value)
### sm
LockRetryTimes = 10
LockRetryInterval = 1s
HealthCheckInterval = 10s
### consul
ConsulBindNetworkName = <adm_nic>
ConsulPort            = <consul_port>
ConsulRetryTimes      = 1
ConsulRetryInterval   = 1s
ConsulRetryTimeout    = 1s
ConsulRetryTimeoutAll = 1s
### swarm
# example DBAAS/DOCKER/SWARM/leader
SwarmHostKey = <swarm_leader_key>
SwarmSocketPath = /DBAASDAT/upsql.sock
SwarmRetryTimes      = 2
SwarmRetryInterval   = 2s
SwarmRetryTimeout    = 2s
SwarmRetryTimeoutAll = 2s
SwarmHealthCheckApp        = /root/check_db
SwarmHealthCheckUser        = check
SwarmHealthCheckPassword    = 123.com
SwarmHealthCheckConfigFile  = /DBAASDAT/my.cnf
SwarmHealthCheckTimeout = 5s
SwarmHealthCheckReadTimeout = 5s
GtidDiff                = 3
GtidDiffRetryTimes      = 3
GtidDiffRetryInterval   = 3s
GtidDiffRetryTimeout    = 3s
GtidDiffRetryTimeoutAll = 3
`

func TestSwitchManagerConfig(t *testing.T) {
	config := new(switchManagerConfig)

	data, err := config.defaultUserConfig(nil, nil)
	if err != nil {
		t.Logf("Expected Error:%s", err.Error())

		data = make(map[string]interface{}, 10)
		data["domain"] = "service_00001"
		data["name"] = "unit_00001"
		data["ProxyPort"] = 9000
		data["Port"] = 8000

		// consul
		data["ConsulBindNetworkName"] = "ConsulBindNetworkName"
		data["SwarmHostKey"] = leaderElectionPath
		data["ConsulPort"] = 4000
	}

	t.Log("Default Config:\n", defaultSWMConfig)

	_, err = config.ParseData([]byte(defaultSWMConfig))
	if err != nil {
		t.Fatal(err)
	}

	err = config.Validate(data)
	if err != nil {
		t.Error(err)
	}

	for key, val := range data {
		if err := config.Set(key, val); err != nil {
			t.Error(err)
		}
	}

	content, err := config.Marshal()
	if err != nil {
		t.Error(err)
	}

	t.Log("config ini:\n", string(content))
}
