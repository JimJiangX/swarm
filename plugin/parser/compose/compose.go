package compose

import (
	"errors"
	"fmt"

	"github.com/docker/swarm/garden/structs"

	log "github.com/Sirupsen/logrus"
)

type DbArch string

const (
	NONE DbArch = "NONE"

	REDIS    DbArch = "REDIS"
	MYSQL_MS DbArch = "MYSQL_MS"
	MYSQL_MG DbArch = "MYSQL_MG"
)

type Composer interface {
	ClearCluster() error
	CheckCluster() error
	ComposeCluster() error
}

func NewCompserBySpec(req *structs.ServiceSpec, mgmip string, mgmport int) (Composer, error) {
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

		return newMysqlComposer(dbs, mgmip, mgmport), nil

	case MYSQL_MG:

	case REDIS:

	case NONE:
	}
	return nil, errors.New("don't support")
}

func getDbType(req *structs.ServiceSpec) DbArch {
	return NONE
}

func getMysqls(req *structs.ServiceSpec) ([]Mysql, error) {

	mysqls := []Mysql{}

	users, err := getMysqlUser(req)
	if err != nil {
		return nil, errors.New("get  mysql users fail:" + err.Error())
	}

	if len(req.Units) == 0 {
		return nil, errors.New("req.Units has no datas")
	}

	for _, unit := range req.Units {
		instance := unit.ContainerID

		if len(unit.Networking.IPs) == 0 || len(unit.Networking.Ports) == 0 {
			errstr := fmt.Sprintf("the unit %s :addr len equal 0", instance)
			return mysqls, errors.New(errstr)
		}

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
