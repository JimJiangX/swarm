package compose

import (
	"errors"

	log "github.com/Sirupsen/logrus"
)

//master-slave redis manager
type RedisMSManager struct {
	RedisMap map[string]Redis
}

func newRedisMSManager(dbs []Redis) Composer {

	ms := &RedisMSManager{
		RedisMap: make(map[string]Redis),
	}

	for _, db := range dbs {
		ms.RedisMap[db.GetKey()] = db
	}

	return ms

}

func (m *RedisMSManager) ComposeCluster() error {
	if err := m.ClearCluster(); err != nil {
		return err
	}

	if err := m.preCompose(); err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Error("preCompose fail")
		return errors.New("preCompose err:" + err.Error())

	}

	masterkey, ok := m.getMasterKey()
	if !ok {
		log.WithFields(log.Fields{
			"error": "don't find the master",
		}).Error("get master key fail")
		return errors.New("don't find the master")
	}

	master := m.RedisMap[masterkey]

	for _, db := range m.RedisMap {
		if db.GetType() != MASTER_TYPE {
			if err := db.ChangeMaster(master); err != nil {
				log.WithFields(log.Fields{
					"slave":  db.GetKey(),
					"master": master.GetKey(),
					"error":  err.Error(),
				}).Error("db ChangeMaster fail")
				return errors.New(db.Ip + ":" + "db ChangeMaster fail")
			}
		}
	}

	return nil
}

func (m *RedisMSManager) ClearCluster() error {
	for _, db := range m.RedisMap {
		if err := db.Clear(); err != nil {
			log.WithFields(log.Fields{
				"db":    db.GetKey(),
				"error": err.Error(),
			}).Error("db Clear fail")
			return errors.New(db.GetKey() + " : clear fail" + "  " + err.Error())
		}
	}

	return nil
}

func (m *RedisMSManager) CheckCluster() error {
	for _, db := range m.RedisMap {
		if err := db.CheckStatus(); err != nil {
			log.WithFields(log.Fields{
				"db":    db.GetKey(),
				"error": err.Error(),
			}).Error("db CheckStatus fail")
			return errors.New(db.GetKey() + " : CheckStatus fail" + "  " + err.Error())
		}
	}
	return nil
}

func (m *RedisMSManager) preCompose() error {
	//select master
	masterkey := m.electMaster()
	if err := m.setType(masterkey, MASTER_TYPE); err != nil {
		return errors.New("electMaster fail:" + err.Error())
	}

	for _, db := range m.RedisMap {
		if db.GetKey() != masterkey {
			if err := m.setType(db.GetKey(), SLAVE_TYPE); err != nil {
				log.WithFields(log.Fields{
					"err": err,
					"db":  db.GetKey(),
				}).Error("set slave fail(should not happen.)")
				return err
			}
		}
	}

	return nil
}

func (m *RedisMSManager) getMasterKey() (string, bool) {
	for _, db := range m.RedisMap {
		if db.GetType() == MASTER_TYPE {
			return db.GetKey(), true
		}
	}

	return "", false
}

func (m *RedisMSManager) electMaster() string {

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

func (m *RedisMSManager) setType(dbkey string, Type ROLE_TYPE) error {

	tmp, ok := m.RedisMap[dbkey]
	if !ok {
		return errors.New("don't find the db key")
	}
	tmp.RoleType = Type

	m.RedisMap[dbkey] = tmp
	return nil

}