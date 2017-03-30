package compose

import (
	"errors"

	log "github.com/Sirupsen/logrus"
)

//master-slave mysql manager
type MysqlMSManager struct {
	Mysqls  map[string]Mysql
	MgmIP   string
	MgmPort int
}

func newMysqlMSManager(dbs []Mysql, ip string, port int) Composer {

	ms := &MysqlMSManager{
		MgmIP:   ip,
		MgmPort: port,
		Mysqls:  make(map[string]Mysql),
	}

	for _, db := range dbs {
		db.MgmIP = ip
		db.MgmPort = port
		ms.Mysqls[db.GetKey()] = db
	}

	return ms

}
func (m *MysqlMSManager) ComposeCluster() error {
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

	master := m.Mysqls[masterkey]

	for _, db := range m.Mysqls {
		if db.GetType() != MASTER_TYPE {
			if err := db.ChangeMaster(master); err != nil {
				log.WithFields(log.Fields{
					"slave":  db.GetKey(),
					"master": master.GetKey(),
					"error":  err.Error(),
				}).Error("db ChangeMaster fail")
				return errors.New(db.IP + ":" + "db ChangeMaster fail")
			}
		}
	}

	return nil
}

func (m *MysqlMSManager) ClearCluster() error {
	for _, db := range m.Mysqls {
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

func (m *MysqlMSManager) CheckCluster() error {
	for _, db := range m.Mysqls {
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

func (m *MysqlMSManager) preCompose() error {
	//select master
	masterkey := m.electMaster()
	if err := m.setMysqlType(masterkey, MASTER_TYPE); err != nil {
		return errors.New("electMaster fail:" + err.Error())
	}

	for _, db := range m.Mysqls {
		if db.GetKey() != masterkey {
			if err := m.setMysqlType(db.GetKey(), SLAVE_TYPE); err != nil {
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

func (m *MysqlMSManager) getMasterKey() (string, bool) {
	for _, db := range m.Mysqls {
		if db.GetType() == MASTER_TYPE {
			return db.GetKey(), true
		}
	}

	return "", false
}

func (m *MysqlMSManager) electMaster() string {

	curweight := -1
	var master Mysql
	tmp := false

	for _, db := range m.Mysqls {
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

func (m *MysqlMSManager) setMysqlType(dbkey string, Type ROLE_TYPE) error {

	tmp, ok := m.Mysqls[dbkey]
	if !ok {
		return errors.New("don't find the db key")
	}
	tmp.RoleType = Type

	m.Mysqls[dbkey] = tmp
	return nil

}
