package compose

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/swarm/garden/structs"

	log "github.com/Sirupsen/logrus"
)

type DbArch string

const (
	BASEDIR string = "/usr/local/mgm/compose/scripts/"

	NONE  DbArch = "None"
	CLONE DbArch = "Clone"

	REDIS_CLUSTER DbArch = "RedisCluster"
	REDIS_MS      DbArch = "RedisMSlave"

	MYSQL_MS DbArch = "MYsqlMSlave"
	MYSQL_MG DbArch = "MysqlGroup"
)

type Composer interface {
	ClearCluster() error
	CheckCluster() error
	ComposeCluster() error
}

func NewCompserBySpec(req *structs.ServiceSpec, mgmip string, mgmport int) (Composer, error) {
	if err := valicateServiceSpec(req); err != nil {
		return nil, errors.New("valicateServiceSpec fail:" + err.Error())
	}

	arch := getDbType(req)
	switch arch {
	case MYSQL_MS:
		dbs, err := getMysqls(req)
		if err != nil {
			log.WithFields(log.Fields{
				"err":  err.Error(),
				"arch": string(MYSQL_MS),
			}).Error("getMysqls")

			errstr := string(MYSQL_MS) + " generate dbs info fail:" + err.Error()
			return nil, errors.New(errstr)
		}

		return newMysqlMSManager(dbs, mgmip, mgmport), nil

	case MYSQL_MG:

	case REDIS_CLUSTER:
		dbs, err := getRedis(req)
		if err != nil {
			log.WithFields(log.Fields{
				"err":  err.Error(),
				"arch": string(REDIS_CLUSTER),
			}).Error("getRedis")

			errstr := string(REDIS_CLUSTER) + " generate dbs info fail:" + err.Error()
			return nil, errors.New(errstr)
		}

		master, slave, _ := getmasterAndSlave(req)
		return newRedisShadeManager(dbs, master, slave), nil

	case NONE:
	}
	return nil, errors.New("don't support")
}

func valicateServiceSpec(req *structs.ServiceSpec) error {
	//req.Image: "mysql:5.6.7"
	datas := strings.Split(req.Image, ":")
	if len(datas) != 2 {
		return errors.New("req.Image:bad format")
	}

	//req.Arch.Code
	_, _, err := getmasterAndSlave(req)
	if err != nil {
		return errors.New("Arch.Code:" + err.Error())
	}

	//unit ip
	for _, unit := range req.Units {
		if len(unit.Networking.IPs) == 0 || len(unit.Networking.Ports) == 0 {
			errstr := fmt.Sprintf("the unit %s :addr len equal 0", unit.ContainerID)
			return errors.New(errstr)
		}

	}

	return nil
}

//check && get value
//"m:2#s:1"
func getmasterAndSlave(req *structs.ServiceSpec) (mnum int, snum int, err error) {
	codes := strings.Split(req.Arch.Code, "#")
	if len(codes) != 2 {
		return 0, 0, errors.New("bad format")
	}

	//get master num
	master := strings.Split(codes[0], ":")
	if len(master) != 2 {
		return 0, 0, errors.New("bad format")
	}
	mnum, err = strconv.Atoi(master[1])
	if err != nil || master[0] != "m" {
		return 0, 0, errors.New("bad format")
	}

	//get slave num
	slave := strings.Split(codes[1], ":")
	if len(slave) != 2 {
		return 0, 0, errors.New("bad format")
	}
	snum, err = strconv.Atoi(slave[1])
	if err != nil || slave[0] != "s" {
		return 0, 0, errors.New("bad format")
	}

	return mnum, snum, nil

}

func getDbType(req *structs.ServiceSpec) DbArch {
	datas := strings.Split(req.Image, ":")
	db, version := datas[0], datas[1]
	arch := req.Arch.Mode

	if db == "redis" && arch == "sharding_replication" {
		return REDIS_CLUSTER
	}

	if db == "redis" && arch == "replication" {
		return REDIS_MS
	}

	if db == "mysql" && arch == "replication" {
		return MYSQL_MS
	}

	if db == "mysql" && arch == "group_replication" {
		return MYSQL_MG
	}

	log.WithFields(log.Fields{
		"db type":    db,
		"db version": version,
		"arch":       arch,
		"err":        "don't match the arch",
	}).Error("getDbType")

	return NONE
}

func getRedis(req *structs.ServiceSpec) ([]Redis, error) {
	redisslice := []Redis{}

	if len(req.Units) == req.Arch.Replicas {
		return nil, errors.New("req.Units not equal with arch.Replicas args")
	}

	for _, unit := range req.Units {

		ip := unit.Networking.IPs[0].IP
		port := unit.Networking.Ports[0].Port

		redis := Redis{
			Ip:   ip,
			Port: port,
		}

		redisslice = append(redisslice, redis)
	}

	return redisslice, nil
}

func getMysqls(req *structs.ServiceSpec) ([]Mysql, error) {

	mysqls := []Mysql{}

	users, err := getMysqlUser(req)
	if err != nil {
		return nil, errors.New("get  mysql users fail:" + err.Error())
	}

	if len(req.Units) == req.Arch.Replicas {
		return nil, errors.New("req.Units not equal with arch.Replicas args")
	}

	for _, unit := range req.Units {
		instance := unit.ContainerID

		ip := unit.Networking.IPs[0].IP
		port := unit.Networking.Ports[0].Port

		mysql := Mysql{
			MysqlUser: users,

			Ip:       ip,
			Port:     port,
			Instance: instance,
		}

		mysqls = append(mysqls, mysql)
	}

	return mysqls, nil
}

func getMysqlUser(req *structs.ServiceSpec) (MysqlUser, error) {
	users := MysqlUser{}
	for _, user := range req.Users {
		_ = user
	}

	if users.Replicatepwd == "" || users.ReplicateUser == "" ||
		users.RootPwd == "" || users.Rootuser == "" {
		return users, errors.New("some fields has no data")
	}

	return users, nil
}
