package compose

import (
	"errors"

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
				"arch": string(MYSQL_MG),
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
	return nil, errors.New("")
}
