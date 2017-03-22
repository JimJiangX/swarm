package compose

import (
	"strconv"
	"time"

	//	log "github.com/Sirupsen/logrus"
)

type Redis struct {
	Ip   string
	Port int
}

func (r Redis) GetKey() string {
	return r.Ip + ":" + strconv.Itoa(r.Port)
}

//master-slave mysql manager
type RedisClusterManager struct {
	RedisMap map[string]Redis

	Master int
	Slave  int
}

func newRedisClusterManager(dbs []Redis, master, slave int) Composer {
	rs := &RedisClusterManager{
		RedisMap: make(map[string]Redis),
	}

	for _, db := range dbs {
		rs.RedisMap[db.GetKey()] = db
	}

	return rs
}

func (r *RedisClusterManager) ClearCluster() error {
	filepath := BASEDIR + ""
	timeout := time.Second * 60
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}

func (r *RedisClusterManager) CheckCluster() error {
	filepath := BASEDIR + ""
	timeout := time.Second * 60
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}

func (r *RedisClusterManager) ComposeCluster() error {
	filepath := BASEDIR + ""
	timeout := time.Second * 120
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}
