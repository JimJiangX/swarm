package store

import (
	"testing"

	"github.com/docker/swarm/cluster/swarm/database"
)

func init() {
	dbSource := "root:111111@tcp(127.0.0.1:3306)/DBaaS?parseTime=true&charset=utf8&loc=Asia%%2FShanghai&sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION'"
	driverName := "mysql"
	database.MustConnect(driverName, dbSource)
}

func TestRegisterStore(t *testing.T) {

	hitachi, err := RegisterStore()

}
