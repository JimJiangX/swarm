package compose

import (
	"errors"
	"strconv"
	"strings"
	"time"

	//	log "github.com/Sirupsen/logrus"
)

//master-slave mysql manager
type RedisShadeManager struct {
	RedisMap map[string]Redis

	Master int
	Slave  int
}

func newRedisShadeManager(dbs []Redis, master int, slave int) Composer {
	rs := &RedisShadeManager{
		RedisMap: make(map[string]Redis),
		Master:   master,
		Slave:    slave,
	}

	for _, db := range dbs {
		rs.RedisMap[db.GetKey()] = db
	}

	return rs
}
func (r *RedisShadeManager) getRedisAddrs() string {
	addrs := []string{}
	for _, redis := range r.RedisMap {
		addr := redis.Ip + ":" + strconv.Itoa(redis.Port)
		addrs = append(addrs, addr)
	}

	return strings.Join(addrs, ",")
}

func (r *RedisShadeManager) ClearCluster() error {
	filepath := BASEDIR + "redis-sharding_replication-reset.sh"
	timeout := time.Second * 120

	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}

func (r *RedisShadeManager) CheckCluster() error {
	return nil
}

func (r *RedisShadeManager) ComposeCluster() error {
	if err := r.ClearCluster(); err != nil {
		return errors.New("ClearCluster fail:" + err.Error())
	}

	filepath := BASEDIR + "redis-sharding_replication-set.sh"
	timeout := time.Second * 120

	addrs := r.getRedisAddrs()
	args := []string{strconv.Itoa(r.Slave), addrs}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}
