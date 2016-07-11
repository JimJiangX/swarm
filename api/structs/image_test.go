package structs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

const (
	keysets      = `{"mysqld::character_set_server":{"Key":"mysqld::character_set_server","CanSet":true,"MustRestart":true,"Description":""},"mysqld::connect_timeout":{"Key":"mysqld::connect_timeout","CanSet":true,"MustRestart":false,"Description":""},"mysqld::interactive_timeout":{"Key":"mysqld::interactive_timeout","CanSet":true,"MustRestart":false,"Description":""},"mysqld::max_connections":{"Key":"mysqld::max_connections","CanSet":true,"MustRestart":false,"Description":""},"mysqld::wait_timeout":{"Key":"mysqld::wait_timeout","CanSet":true,"MustRestart":false,"Description":""}} `
	upsqlContent = "################\n##UpSQL 5.6.19##\n################\n[mysqld]\nbind-address =  <ip_addr>\nport = <port>\nsocket = /DBAASDAT/upsql.sock\nserver-id = <port>\ncharacter_set_server = <char_set>\nmax_connect_errors = 50000\nmax_connections = 5000\nmax_user_connections = 0\n#skip-name-resolve setting in cmd option\nskip_external_locking = ON\nmax_allowed_packet = 16M\nsort_buffer_size = 2M\njoin_buffer_size = 128K\nuser = upsql\ntmpdir = /DBAASDAT\ndatadir = /DBAASDAT\nlog-bin = /DBAASLOG/BIN/<container_name>-binlog\nlog_bin_trust_function_creators = ON\nsync_binlog = 1\nexpire_logs_days = 0\nkey_buffer_size = 160M\nbinlog_cache_size = 1M\nbinlog_format = row\nlower_case_table_names = 1\nmax_binlog_size = 1G\nconnect_timeout = 60\ninteractive_timeout = 31536000\nwait_timeout = 31536000\nnet_read_timeout = 30\nnet_write_timeout = 60\noptimizer_switch = 'mrr=on,mrr_cost_based=off'\nopen_files_limit = 10240\nexplicit_defaults_for_timestamp = true\ninnodb_open_files = 1024\ninnodb_data_file_path=ibdata1:12M:autoextend\ninnodb_buffer_pool_size = <container_mem> * 0.75\ninnodb_buffer_pool_instances = 8\ninnodb_log_buffer_size = 128M\ninnodb_log_file_size = 128M\ninnodb_log_files_in_group = 7\ninnodb_log_group_home_dir = /DBAASLOG/RED\ninnodb_max_dirty_pages_pct = 30\ninnodb_flush_method = O_DIRECT\ninnodb_flush_log_at_trx_commit = 1\ninnodb_thread_concurrency = 16\ninnodb_read_io_threads = 4\ninnodb_write_io_threads = 4\ninnodb_lock_wait_timeout = 60\ninnodb_rollback_on_timeout = on\ninnodb_file_per_table = 1\ninnodb_stats_sample_pages = 1\ninnodb_purge_threads = 1\ninnodb_stats_on_metadata = OFF\ninnodb_support_xa = 1\ninnodb_doublewrite = 1\ninnodb_checksums = 1\ninnodb_io_capacity = 500\ninnodb_purge_threads = 8\ninnodb_purge_batch_size = 500\ninnodb_stats_persistent_sample_pages = 10\nplugin_dir = /usr/local/mysql/lib/plugin\nplugin_load = \"rpl_semi_sync_master=semisync_master.so;rpl_semi_sync_slave=semisync_slave.so;upsql_auth=upsql_auth.so\"\nloose_rpl_semi_sync_master_enabled = 1\nloose_rpl_semi_sync_slave_enabled = 1\n##[Replication variables]\ngtid-mode = on\nenforce-gtid-consistency = on\nlog-slave-updates = on\nbinlog_checksum = CRC32\nbinlog_row_image = minimal\nslave_sql_verify_checksum = on\nslave_parallel_workers = 5\nmaster_verify_checksum  =   ON\nslave_sql_verify_checksum = ON\nmaster_info_repository=TABLE\nrelay_log_info_repository=TABLE\nreplicate-ignore-db=dbaas_check\n##[Replication variables for Master]\nrpl_semi_sync_master_enabled = on\nauto_increment_increment = 1\nauto_increment_offset = 1\nrpl_semi_sync_master_timeout = 10000\nrpl_semi_sync_master_wait_no_slave = on\nrpl_semi_sync_master_trace_level = 32\n##[Replication variables for Slave]\nrpl_semi_sync_slave_enabled = on\nrpl_semi_sync_slave_trace_level = 32\nslave_net_timeout = 10\nrelay_log_recovery = on\nlog_slave_updates = on\nmax_relay_log_size = 1G\nrelay_log = /DBAASLOG/REL/<container_name>-relay\nrelay_log_purge = on\n[mysqldump]\n#quick\nmax_allowed_packet = 16M\n[myisamchk]\nkey_buffer_size = 20M\nsort_buffer_size = 2M\nread_buffer = 2M\nwrite_buffer = 2M\n[mysqlhotcopy]\n#interactive-timeou\n\n"
	upsqlText    = `
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

func iniParse(content, keysets string) ([]ValueAndKeyset, error) {
	type keySet struct {
		Key         string
		CanSet      bool
		MustRestart bool
		Description string
	}
	var (
		delimiter  = "::"
		prefix     = "default"
		kvs        = make([]ValueAndKeyset, 0, 100)
		keysetsMap map[string]keySet
	)

	err := json.Unmarshal([]byte(keysets), &keysetsMap)
	if err != nil {
		return nil, err
	}

	fmt.Println(keysetsMap)

	lenContent := len(content)
	lenKV := 0
	buf := bytes.NewBufferString(content)
	for {
		if lenKV == lenContent {
			break
		}
		s, err := buf.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		lenKV += len(s)
		s = strings.TrimSpace(s)
		if strings.Index(s, "[") == 0 {
			prefix = s[1 : len(s)-1]
			continue
		}
		if strings.Index(s, "#") == 0 {
			continue
		}
		index := strings.Index(s, "=")
		if index < 0 {
			continue
		}
		key := strings.TrimSpace(s[:index])
		if len(key) == 0 {
			continue
		}
		val := strings.TrimSpace(s[index+1:])
		if len(val) == 0 {
			continue
		}
		index = strings.Index(val, "#")
		if index > 0 {
			val = strings.TrimSpace(val[:index])
		}

		key = prefix + delimiter + key
		v := keysetsMap[key]
		kvs = append(kvs, ValueAndKeyset{
			Value: val,
			KeysetParams: KeysetParams{
				Key:         key,
				CanSet:      v.CanSet,
				MustRestart: v.MustRestart,
				Description: v.Description,
			},
		})
	}

	return kvs, nil
}

func TestIniParse(t *testing.T) {
	val, err := iniParse(upsqlContent, keysets)
	if err != nil {
		t.Fatal(err)
	}

	for i := range val {
		t.Log(val[i])
	}
}
