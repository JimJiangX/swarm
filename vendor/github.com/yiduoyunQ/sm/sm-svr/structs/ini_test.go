package structs

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

const ini = `
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

func TestReadIniFileByLine(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(ini))
	for {
		key, val, err := ReadIniFileByLine(r)
		if err != nil {
			if err == io.EOF {
				break
			}

			t.Error(err)
		} else {
			t.Log(key, val)
		}
	}
}
