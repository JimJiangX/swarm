package parser

import (
	"testing"
)

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

func TestParseRedisConfig(t *testing.T) {
	config, err := parseRedisConfig([]byte(redisTemplateContext))
	if err != nil {
		t.Error(err)
	}

	if len(config) != 51 {
		t.Error(len(config))
	}
}
