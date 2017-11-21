package compose

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/vars"
	"github.com/pkg/errors"
)

type dbArch string
type dbRole string

const (
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

//Composer is exported
type Composer interface {
	ClearCluster() error
	CheckCluster() error
	ComposeCluster() error
}

//get related Composer  by ServiceSpec
func NewCompserBySpec(req *structs.ServiceSpec, script, mgmip string, mgmport int) (Composer, error) {

	if err := valicateServiceSpec(req); err != nil {
		return nil, err
	}

	arch := getDbType(req)

	switch arch {
	case mysqlRepArch, mysqlGroupArch:
		dbs := getMysqls(req)
		return newMysqlRepManager(dbs, script, mgmip, mgmport), nil

	case redisShardingArch:
		dbs := getRedis(req)
		master, slave, _ := getmasterAndSlave(req)
		return newRedisShardingManager(dbs, master, slave, script), nil

	case redisRepArch:
		dbs := getRedis(req)

		return newRedisRepManager(dbs, script), nil

	case cloneArch:
		return newCloneManager(script), nil
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
	errs := make([]string, 0, 4)
	//req.Image: "mysql:5.6.7"
	if req.Image.Name == "" {
		errs = append(errs, "image name is required")
	}

	//req.Arch.Code
	_, _, err := getmasterAndSlave(req)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%+v", err))
	}

	if len(req.Units) != req.Arch.Replicas {
		errs = append(errs, "req.Units nums not equal with req.Arch.Replicas")
	}

	//unit ip
	for _, unit := range req.Units {
		if len(unit.Networking) == 0 {
			errs = append(errs, fmt.Sprintf("the unit %s :addr len equal 0", unit.ContainerID))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("%s", strings.Join(errs, "\n"))
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

	return -1, errors.Errorf("unknown port type,%s", port)
}

func getRedisPortBySpec(req *structs.ServiceSpec) (int, error) {
	port, ok := req.Options["port"]
	if !ok {
		return -1, errors.New("bad req:redis need Options[port]")
	}

	return convertPort(port)
}

func getMysqlPortBySpec(req *structs.ServiceSpec) (int, error) {

	port, ok := req.Options["mysqld::port"]
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
//"M:2#SB:1#S:1"
func getmasterAndSlave(req *structs.ServiceSpec) (int, int, error) {
	codes := strings.Split(req.Arch.Code, "#")

	//get master num
	master := strings.Split(codes[0], ":")
	if len(master) != 2 {
		return 0, 0, errors.Errorf("bad format,get master,Arch.Code:%v", req.Arch.Code)
	}
	mnum, err := strconv.Atoi(master[1])
	if err != nil || master[0] != "M" {
		return 0, 0, errors.Errorf("bad format,get master num,Arch.Code:%v", req.Arch.Code)
	}

	if len(codes) == 1 {
		return mnum, 0, nil
	}

	//get slave num
	slave := strings.Split(codes[1], ":")
	if len(slave) != 2 {
		return 0, 0, errors.Errorf("bad format,get slave,Arch.Code:%v", req.Arch.Code)
	}
	snum, err := strconv.Atoi(slave[1])
	if err != nil || (slave[0] != "S" && slave[0] != "SB") {
		return 0, 0, errors.Errorf("bad format,get slave num,Arch.Code:%v", req.Arch.Code)
	}

	if len(codes) == 1 {
		return mnum, snum, nil
	}

	slave = strings.Split(codes[2], ":")
	if len(slave) != 2 {
		return 0, 0, errors.Errorf("bad format,get slave,Arch.Code:%v", req.Arch.Code)
	}
	n, err := strconv.Atoi(slave[2])
	if err != nil || (slave[0] != "S" && slave[0] != "SB") {
		return 0, 0, errors.Errorf("bad format,get slave num,Arch.Code:%v", req.Arch.Code)
	}

	return mnum, snum + n, nil
}

func getDbType(req *structs.ServiceSpec) dbArch {
	arch := req.Arch.Mode

	if req.Image.Name == "redis" || req.Image.Name == "upredis" {
		if arch == "sharding_replication" {
			return redisShardingArch
		} else if arch == "replication" {
			return redisRepArch
		}
	}

	if req.Image.Name == "mysql" || req.Image.Name == "upsql" {
		if arch == "replication" {
			return mysqlRepArch
		} else if arch == "group_replication" {
			return mysqlGroupArch
		}
	}

	if arch == "clone" {
		return cloneArch
	}

	logrus.WithFields(logrus.Fields{
		"image": req.Image.Image(),
		"arch":  arch,
	}).Error("don't match the arch")

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
	user, err := getMysqlUser(req)
	if err != nil {
		logrus.Warnf("%+v", err)
	}

	intport, err := getMysqlPortBySpec(req)
	if err != nil {
		logrus.Warnf("%+v", err)
	}

	mysqls := make([]Mysql, 0, len(req.Units))

	for _, unit := range req.Units {
		instance := unit.ContainerID

		ip := unit.Networking[0].IP

		mysql := Mysql{
			user:     user,
			IP:       ip,
			Port:     intport,
			Instance: instance,
		}

		mysqls = append(mysqls, mysql)
	}

	return mysqls
}

func getMysqlUser(req *structs.ServiceSpec) (mysqlUser, error) {
	user := mysqlUser{
		user:     vars.Replication.User,
		password: vars.Replication.Password,
	}

	if user.password == "" || user.user == "" {
		return mysqlUser{}, errors.New("mysql replication pwd/user has no data")
	}

	return user, nil
}
