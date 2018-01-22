package compose

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/utils"
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
	addrs := r.getRedisAddrs()
	args := []string{filepath, addrs}

	out, err := utils.ExecContextTimeout(nil, defaultTimeout*2, args...)

	logrus.Debugf("exec:%s,output:%s", args, out)

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

	addrs := r.getRedisAddrs()
	args := []string{filepath, strconv.Itoa(r.Slave), addrs}

	out, err := utils.ExecContextTimeout(nil, defaultTimeout*2, args...)

	logrus.Debugf("exec:%s,output:%s", args, out)

	return err
}
