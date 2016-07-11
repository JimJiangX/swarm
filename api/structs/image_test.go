package structs

import (
	"testing"
)

const (
	keysets      = `{"mysqld::character_set_server":{"Key":"mysqld::character_set_server","CanSet":true,"MustRestart":true,"Description":""},"mysqld::connect_timeout":{"Key":"mysqld::connect_timeout","CanSet":true,"MustRestart":false,"Description":""},"mysqld::interactive_timeout":{"Key":"mysqld::interactive_timeout","CanSet":true,"MustRestart":false,"Description":""},"mysqld::max_connections":{"Key":"mysqld::max_connections","CanSet":true,"MustRestart":false,"Description":""},"mysqld::wait_timeout":{"Key":"mysqld::wait_timeout","CanSet":true,"MustRestart":false,"Description":""}}`
	upsqlContent = `#UpSQL 5.6.19## # [mysqld] replicate-ignore-db=dbaas_check rpl_semi_sync_slave_trace_level=32 log-bin=/DBAASLOG/BIN/5b9540a2_abc_01-binlog sync_binlog=1 innodb_read_io_threads=4 binlog_row_image=minimal loose_rpl_semi_sync_slave_enabled=1 slave_sql_verify_checksum=ON #[Replication variables for Master] rpl_semi_sync_master_enabled=on port=30004 #skip-name-resolve setting in cmd option skip_external_locking=ON expire_logs_days=0 net_write_timeout=60 open_files_limit=10240 innodb_log_buffer_size=128M bind-address=192.168.20.52 server-id=30004 datadir=/DBAASDAT connect_timeout=60 innodb_purge_batch_size=500 rpl_semi_sync_master_timeout=10000 rpl_semi_sync_master_trace_level=32 relay_log_purge=on user=upsql innodb_stats_sample_pages=1 innodb_purge_threads=8 innodb_doublewrite=1 relay_log_info_repository=TABLE socket=/DBAASDAT/upsql.sock join_buffer_size=128K tmpdir=/DBAASDAT interactive_timeout=31536000 #[Replication variables] gtid-mode=on auto_increment_offset=1 wait_timeout=31536000 innodb_log_group_home_dir=/DBAASLOG/RED innodb_flush_method=O_DIRECT innodb_flush_log_at_trx_commit=1 relay_log=/DBAASLOG/REL/5b9540a2_abc_01-relay max_allowed_packet=16M key_buffer_size=160M innodb_buffer_pool_instances=8 innodb_log_files_in_group=7 innodb_data_file_path=ibdata1:12M:autoextend innodb_file_per_table=1 innodb_support_xa=1 max_relay_log_size=1G explicit_defaults_for_timestamp=true innodb_io_capacity=500 rpl_semi_sync_master_wait_no_slave=on max_connections=5000 max_user_connections=0 binlog_format=row max_binlog_size=1G relay_log_recovery=on log_slave_updates=on innodb_open_files=1024 innodb_log_file_size=128M innodb_rollback_on_timeout=on loose_rpl_semi_sync_master_enabled=1 binlog_cache_size=1M innodb_max_dirty_pages_pct=30 master_info_repository=TABLE auto_increment_increment=1 innodb_stats_on_metadata=OFF plugin_dir=/usr/local/mysql/lib/plugin plugin_load=rpl_semi_sync_master=semisync_master.so;rpl_semi_sync_slave=semisync_slave.so;upsql_auth=upsql_auth.so master_verify_checksum=ON log_bin_trust_function_creators=ON optimizer_switch='mrr=on,mrr_cost_based=off' innodb_buffer_pool_size=805306368 innodb_lock_wait_timeout=60 slave_parallel_workers=5 character_set_server=utf8 innodb_thread_concurrency=16 innodb_write_io_threads=4 innodb_checksums=1 innodb_stats_persistent_sample_pages=10 #[Replication variables for Slave] rpl_semi_sync_slave_enabled=on net_read_timeout=30 slave_net_timeout=10 log-slave-updates=on binlog_checksum=CRC32 max_connect_errors=50000 sort_buffer_size=2M lower_case_table_names=1 enforce-gtid-consistency=on  [mysqldump] #quick max_allowed_packet=16M  [myisamchk] sort_buffer_size=2M read_buffer=2M write_buffer=2M key_buffer_size=20M  [mysqlhotcopy]
`
	upsqlText = `
#UpSQL 5.6.19##
#
[mysqld]
replicate-ignore-db=dbaas_check
rpl_semi_sync_slave_trace_level=32
log-bin=/DBAASLOG/BIN/5b9540a2_abc_01-binlog
sync_binlog=1
innodb_read_io_threads=4
binlog_row_image=minimal
loose_rpl_semi_sync_slave_enabled=1
slave_sql_verify_checksum=ON
#[Replication variables for Master]
rpl_semi_sync_master_enabled=on
port=30004
#skip-name-resolve setting in cmd option
skip_external_locking=ON
expire_logs_days=0
net_write_timeout=60
open_files_limit=10240
innodb_log_buffer_size=128M
bind-address=192.168.20.52
server-id=30004
datadir=/DBAASDAT
connect_timeout=60
innodb_purge_batch_size=500
rpl_semi_sync_master_timeout=10000
rpl_semi_sync_master_trace_level=32
relay_log_purge=on
user=upsql
innodb_stats_sample_pages=1
innodb_purge_threads=8
innodb_doublewrite=1
relay_log_info_repository=TABLE
socket=/DBAASDAT/upsql.sock
join_buffer_size=128K
tmpdir=/DBAASDAT
interactive_timeout=31536000
#[Replication variables]
gtid-mode=on
auto_increment_offset=1
wait_timeout=31536000
innodb_log_group_home_dir=/DBAASLOG/RED
innodb_flush_method=O_DIRECT
innodb_flush_log_at_trx_commit=1
relay_log=/DBAASLOG/REL/5b9540a2_abc_01-relay
max_allowed_packet=16M
key_buffer_size=160M
innodb_buffer_pool_instances=8
innodb_log_files_in_group=7
innodb_data_file_path=ibdata1:12M:autoextend
innodb_file_per_table=1
innodb_support_xa=1
max_relay_log_size=1G
explicit_defaults_for_timestamp=true
innodb_io_capacity=500
rpl_semi_sync_master_wait_no_slave=on
max_connections=5000
max_user_connections=0
binlog_format=row
max_binlog_size=1G
relay_log_recovery=on
log_slave_updates=on
innodb_open_files=1024
innodb_log_file_size=128M
innodb_rollback_on_timeout=on
loose_rpl_semi_sync_master_enabled=1
binlog_cache_size=1M
innodb_max_dirty_pages_pct=30
master_info_repository=TABLE
auto_increment_increment=1
innodb_stats_on_metadata=OFF
plugin_dir=/usr/local/mysql/lib/plugin
plugin_load=rpl_semi_sync_master=semisync_master.so;rpl_semi_sync_slave=semisync_slave.so;upsql_auth=upsql_auth.so
master_verify_checksum=ON
log_bin_trust_function_creators=ON
optimizer_switch='mrr=on,mrr_cost_based=off'
innodb_buffer_pool_size=805306368
innodb_lock_wait_timeout=60
slave_parallel_workers=5
character_set_server=utf8
innodb_thread_concurrency=16
innodb_write_io_threads=4
innodb_checksums=1
innodb_stats_persistent_sample_pages=10
#[Replication variables for Slave]
rpl_semi_sync_slave_enabled=on
net_read_timeout=30
slave_net_timeout=10
log-slave-updates=on
binlog_checksum=CRC32
max_connect_errors=50000
sort_buffer_size=2M
lower_case_table_names=1
enforce-gtid-consistency=on

[mysqldump]
#quick
max_allowed_packet=16M

[myisamchk]
sort_buffer_size=2M
read_buffer=2M
write_buffer=2M
key_buffer_size=20M

[mysqlhotcopy]


`
)

/*
type KeysetParams struct {
	Key         string // mysqld::sync_binlog
	CanSet      bool   `json:"can_set"`
	MustRestart bool   `json:"must_restart"`
	Description string `json:",omitempty"`
}

type ValueAndKeyset struct {
	Value string
	KeysetParams
}
*/

func parse(content, keysets string) ([]ValueAndKeyset, error) {
	return nil, nil
}

func TestParse(t *testing.T) {
	val, err := parse(upsqlContent, keysets)
	if err != nil {
		t.Error(err)
	}

	t.Log(val)
}
