package compose

import (
	"errors"

	"github.com/docker/swarm/garden/structs"
)

type Composer interface {
	ClearCluster() error
	CheckCluster() error
	ComposeCluster() error
}

func NewCompserBySpec(req structs.ServiceSpec, mgmip string, mgmport int) (Composer, error) {
	return nil, errors.New("don't support")
}

func newMysqlComposer(arch string, dbs []Mysql, mgmIp string, mgmPort int) (Composer, error) {
	if arch == "MS" {
		ms := &MysqlMSManager{
			MgmIp:   mgmIp,
			MgmPort: mgmPort,
			Mysqls:  make(map[string]Mysql),
		}

		for _, db := range dbs {
			db.MgmIp = mgmIp
			db.MgmPort = mgmPort
			ms.Mysqls[db.GetKey()] = db
		}

		return ms, nil
	} /*else if arch == "MG" {

	}*/

	return nil, errors.New("mysql:don't support arch")

}
