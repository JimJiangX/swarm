package compose

import (
	"github.com/pkg/errors"
)

//master-slave redis manager
type RedisRepManager struct {
	RedisMap map[string]Redis
}

func newRedisRepManager(dbs []Redis) Composer {

	ms := &RedisRepManager{
		RedisMap: make(map[string]Redis),
	}

	for _, db := range dbs {
		ms.RedisMap[db.GetKey()] = db
	}

	return ms

}

func (m *RedisRepManager) ComposeCluster() error {
	err := m.ClearCluster()
	if err != nil {
		return err
	}

	if err := m.preCompose(); err != nil {
		return err
	}

	masterkey, ok := m.getMasterKey()
	if !ok {
		return errors.New("don't find the master")
	}

	master := m.RedisMap[masterkey]

	for _, db := range m.RedisMap {
		if db.GetType() != masterRole {
			if err := db.ChangeMaster(master); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *RedisRepManager) ClearCluster() error {
	for _, db := range m.RedisMap {
		if err := db.Clear(); err != nil {
			return err
		}
	}

	return nil
}

func (m *RedisRepManager) CheckCluster() error {
	for _, db := range m.RedisMap {
		if err := db.CheckStatus(); err != nil {
			return err
		}
	}
	return nil
}

func (m *RedisRepManager) preCompose() error {
	//select master
	masterkey := m.electMaster()
	if err := m.setType(masterkey, masterRole); err != nil {
		return errors.New("electMaster fail:" + err.Error())
	}

	for _, db := range m.RedisMap {
		if db.GetKey() != masterkey {
			if err := m.setType(db.GetKey(), slaveRole); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *RedisRepManager) getMasterKey() (string, bool) {
	for _, db := range m.RedisMap {
		if db.GetType() == masterRole {
			return db.GetKey(), true
		}
	}

	return "", false
}

func (m *RedisRepManager) electMaster() string {

	curweight := -1
	var master Redis
	tmp := false

	for _, db := range m.RedisMap {
		if db.Weight > curweight {
			tmp = true
			master = db
			curweight = db.Weight
		}
	}

	if tmp {
		return master.GetKey()
	}

	return ""
}

func (m *RedisRepManager) setType(dbkey string, Type dbRole) error {
	tmp, ok := m.RedisMap[dbkey]
	if !ok {
		return errors.New("don't find the db key")
	}
	tmp.RoleType = Type

	m.RedisMap[dbkey] = tmp

	return nil

}
