package compose

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/utils"
)

//master-slave redis manager
type RedisRepManager struct {
	RedisMap map[string]Redis

	scriptDir string
}

func newRedisRepManager(dbs []Redis, dir string) Composer {
	ms := &RedisRepManager{
		RedisMap:  make(map[string]Redis),
		scriptDir: dir,
	}

	for _, db := range dbs {
		ms.RedisMap[db.GetKey()] = db
	}

	return ms
}

func (m *RedisRepManager) getRedisAddrs() []string {
	addrs := make([]string, 0, len(m.RedisMap))

	for _, r := range m.RedisMap {
		addrs = append(addrs, fmt.Sprintf("%s:%d", r.Ip, r.Port))
	}

	return addrs
}

func (m *RedisRepManager) ComposeCluster() error {
	err := m.ClearCluster()
	if err != nil {
		return err
	}

	path := filepath.Join(m.scriptDir, "upredis-replication-set.sh")

	addrs := m.getRedisAddrs()
	args := []string{path, strings.Join(addrs, ",")}

	out, err := utils.ExecContextTimeout(nil, defaultTimeout*2, args...)

	logrus.Debugf("exec:%s,output:%s", args, out)

	return err
}

func (m *RedisRepManager) ClearCluster() error {
	path := filepath.Join(m.scriptDir, "upredis-replication-reset.sh")
	addrs := m.getRedisAddrs()
	args := []string{path, strings.Join(addrs, ",")}

	out, err := utils.ExecContextTimeout(nil, defaultTimeout*2, args...)

	logrus.Debugf("exec:%s,output:%s", args, out)

	return err
}

func (m *RedisRepManager) CheckCluster() error {

	return nil
}

//func (m *RedisRepManager) preCompose() error {
//	//select master
//	masterkey := m.electMaster()
//	if err := m.setType(masterkey, masterRole); err != nil {
//		return errors.New("electMaster fail:" + err.Error())
//	}

//	for _, db := range m.RedisMap {
//		if db.GetKey() != masterkey {
//			if err := m.setType(db.GetKey(), slaveRole); err != nil {
//				return err
//			}
//		}
//	}

//	return nil
//}

//func (m *RedisRepManager) getMasterKey() (string, bool) {
//	for _, db := range m.RedisMap {
//		if db.GetType() == masterRole {
//			return db.GetKey(), true
//		}
//	}

//	return "", false
//}

//func (m *RedisRepManager) electMaster() string {

//	curweight := -1
//	var master Redis
//	tmp := false

//	for _, db := range m.RedisMap {
//		if db.Weight > curweight {
//			tmp = true
//			master = db
//			curweight = db.Weight
//		}
//	}

//	if tmp {
//		return master.GetKey()
//	}

//	return ""
//}

//func (m *RedisRepManager) setType(dbkey string, Type dbRole) error {
//	tmp, ok := m.RedisMap[dbkey]
//	if !ok {
//		return errors.New("don't find the db key")
//	}
//	tmp.RoleType = Type

//	m.RedisMap[dbkey] = tmp

//	return nil

//}
