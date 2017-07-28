package compose

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//master-slave redis manager
type RedisShardingManager struct {
	RedisMap map[string]Redis

	Master int
	Slave  int

	scriptDir string
}

func newRedisShardingManager(dbs []Redis, master int, slave int, dir string) Composer {
	rs := &RedisShardingManager{
		RedisMap:  make(map[string]Redis),
		Master:    master,
		Slave:     slave,
		scriptDir: dir,
	}

	for _, db := range dbs {
		db.scriptDir = dir
		rs.RedisMap[db.GetKey()] = db
	}

	return rs
}
func (r *RedisShardingManager) getRedisAddrs() string {
	addrs := []string{}
	for _, redis := range r.RedisMap {
		addr := redis.Ip + ":" + strconv.Itoa(redis.Port)
		addrs = append(addrs, addr)
	}

	return strings.Join(addrs, ",")
}

func (r *RedisShardingManager) ClearCluster() error {
	filepath := filepath.Join(r.scriptDir, "redis-sharding_replication-reset.sh")
	timeout := time.Second * 120
	addrs := r.getRedisAddrs()
	args := []string{addrs}

	_, err := ExecShellFileTimeout(filepath, timeout, args...)

	return err
}

func (r *RedisShardingManager) CheckCluster() error {
	return nil
}

func (r *RedisShardingManager) ComposeCluster() error {
	err := r.ClearCluster()
	if err != nil {
		return err
	}

	filepath := filepath.Join(r.scriptDir, "redis-sharding_replication-set.sh")
	timeout := time.Second * 120

	addrs := r.getRedisAddrs()
	args := []string{strconv.Itoa(r.Slave), addrs}

	_, err = ExecShellFileTimeout(filepath, timeout, args...)

	return err
}
