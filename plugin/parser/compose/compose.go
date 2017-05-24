package compose

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/structs"
)

type dbArch string
type dbRole string

const (
	scriptDir string = "./script/plugin/compose/"

	//db cluster arch
	noneArch  dbArch = "None"
	cloneArch dbArch = "clone"

	redisShardingArch dbArch = "redis_sharding_replication"
	redisRepArch      dbArch = "redis_replication"

	mysqlRepArch   dbArch = "MYsqlMSlave"
	mysqlGroupArch dbArch = "MysqlGroup"

	//db role type
	shardingRole dbRole = "SHARDING"
	groupRole    dbRole = "GROUP"
	masterRole   dbRole = "MASTER"
	slaveRole    dbRole = "SLAVE"
)

//Composer  is exported
type Composer interface {
	ClearCluster() error
	CheckCluster() error
	ComposeCluster() error
}

//get related Composer  by ServiceSpec
func NewCompserBySpec(req *structs.ServiceSpec, mgmip string, mgmport int) (Composer, error) {

	if err := valicateServiceSpec(req); err != nil {
		return nil, errors.New("valicateServiceSpec fail:" + err.Error())
	}

	arch := getDbType(req)

	switch arch {
	case mysqlRepArch, mysqlGroupArch:
		dbs := getMysqls(req)
		return newMysqlRepManager(dbs, mgmip, mgmport), nil

	case redisShardingArch:
		dbs := getRedis(req)
		master, slave, _ := getmasterAndSlave(req)
		return newRedisShardingManager(dbs, master, slave), nil

	case cloneArch:
		return newCloneManager(), nil
	}

	return nil, errors.New(string(arch) + ":the composer do not implement yet")
}

func valicateServiceSpec(req *structs.ServiceSpec) error {
	if err := valicateCommonSpec(req); err != nil {
		return err
	}

	arch := getDbType(req)
	switch arch {
	case mysqlRepArch, mysqlGroupArch:
		return valicateMysqlSpec(req)

	case redisShardingArch, redisRepArch:
		return valicateRedisSpec(req)

	case cloneArch:
		return nil

	case noneArch:
		return errors.New("not support the arch")
	}

	return nil
}

func valicateCommonSpec(req *structs.ServiceSpec) error {
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

	if len(req.Units) != req.Arch.Replicas {
		return errors.New("req.Units nums not equal with req.Arch.Replicas ")
	}
	//unit ip
	for _, unit := range req.Units {
		if len(unit.Networking) == 0 {
			errstr := fmt.Sprintf("the unit %s :addr len equal 0", unit.ContainerID)
			return errors.New(errstr)
		}
	}

	return nil
}

func valicateMysqlSpec(req *structs.ServiceSpec) error {
	//mysql user
	_, err := getMysqlUser(req)
	if err != nil {
		return err
	}

	//mysql port
	if _, err := getMysqlPortBySpec(req); err != nil {
		return err
	}

	for _, unit := range req.Units {
		if unit.ContainerID == "" {
			return errors.New("bad req:mysql ContainerID empty.")
		}
	}

	return nil

}

func convertPort(port interface{}) (int, error) {

	switch value := port.(type) {

	case int:
		return value, nil
	case float64:
		return int(value), nil
	case float32:
		return int(value), nil
	case int64:
		return int(value), nil
	case string:
		return strconv.Atoi(value)
	case int32:
		return int(value), nil
	default:
	}

	return -1, errors.New("unknown port type")
}

func getRedisPortBySpec(req *structs.ServiceSpec) (int, error) {
	port, ok := req.Options["port"]
	if !ok {
		return -1, errors.New("bad req:redis need Options[port]")
	}

	return convertPort(port)
}

func getMysqlPortBySpec(req *structs.ServiceSpec) (int, error) {

	port, ok := req.Options["port"]
	if !ok {
		return -1, errors.New("bad req:mysql need Options[port]")
	}

	return convertPort(port)
}

func valicateRedisSpec(req *structs.ServiceSpec) error {
	_, err := getRedisPortBySpec(req)
	return err
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
	if err != nil || master[0] != "M" {
		return 0, 0, errors.New("bad format")
	}

	//get slave num
	slave := strings.Split(codes[1], ":")
	if len(slave) != 2 {
		return 0, 0, errors.New("bad format")
	}
	snum, err = strconv.Atoi(slave[1])
	if err != nil || slave[0] != "S" {
		return 0, 0, errors.New("bad format")
	}

	return mnum, snum, nil

}

func getDbType(req *structs.ServiceSpec) dbArch {
	datas := strings.Split(req.Image, ":")
	db, version := datas[0], datas[1]
	arch := req.Arch.Mode

	if db == "redis" && arch == "sharding_replication" {
		return redisShardingArch
	}

	if db == "redis" && arch == "replication" {
		return redisRepArch
	}

	if db == "mysql" && arch == "replication" {
		return mysqlRepArch
	}

	if db == "mysql" && arch == "group_replication" {
		return mysqlGroupArch
	}

	if arch == "clone" {
		return cloneArch
	}

	log.WithFields(log.Fields{
		"db type":    db,
		"db version": version,
		"arch":       arch,
		"err":        "don't match the arch",
	}).Error("getDbType")

	return noneArch
}

func getRedis(req *structs.ServiceSpec) []Redis {
	redisslice := []Redis{}

	for _, unit := range req.Units {

		ip := unit.Networking[0].IP
		port, _ := getRedisPortBySpec(req)

		redis := Redis{
			Ip:   ip,
			Port: port,
		}

		redisslice = append(redisslice, redis)
	}

	return redisslice
}

func getMysqls(req *structs.ServiceSpec) []Mysql {

	mysqls := []Mysql{}

	users, _ := getMysqlUser(req)

	intport, _ := getMysqlPortBySpec(req)

	for _, unit := range req.Units {
		instance := unit.ContainerID

		ip := unit.Networking[0].IP

		mysql := Mysql{
			MysqlUser: users,

			IP:       ip,
			Port:     intport,
			Instance: instance,
		}

		mysqls = append(mysqls, mysql)
	}

	return mysqls
}

func getMysqlUser(req *structs.ServiceSpec) (MysqlUser, error) {
	users := MysqlUser{}

	for _, user := range req.Users {
		if user.Role == "replication" {
			users.Replicatepwd = user.Password
			users.ReplicateUser = user.Name
		}
	}

	if users.Replicatepwd == "" || users.ReplicateUser == "" {
		return users, errors.New("bad req: mysql replication pwd/user has no data")
	}

	return users, nil
}
