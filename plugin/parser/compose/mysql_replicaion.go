package compose

import (
	"github.com/pkg/errors"
)

//master-slave mysql manager
type MysqlRepManager struct {
	Mysqls  map[string]Mysql
	MgmIP   string
	MgmPort int
}

func newMysqlRepManager(dbs []Mysql, dir, ip string, port int) Composer {

	ms := &MysqlRepManager{
		MgmIP:   ip,
		MgmPort: port,
		Mysqls:  make(map[string]Mysql),
	}

	for _, db := range dbs {
		db.MgmIP = ip
		db.MgmPort = port
		db.scriptDir = dir
		ms.Mysqls[db.GetKey()] = db
	}

	return ms

}
func (m *MysqlRepManager) ComposeCluster() error {
	if err := m.ClearCluster(); err != nil {
		return err
	}

	if err := m.preCompose(); err != nil {
		return err
	}

	masterkey, ok := m.getMasterKey()
	if !ok {
		return errors.New("don't find the master")
	}

	master := m.Mysqls[masterkey]

	for _, db := range m.Mysqls {
		if db.GetType() != masterRole {
			if err := db.ChangeMaster(master); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *MysqlRepManager) ClearCluster() error {
	for _, db := range m.Mysqls {
		if err := db.Clear(); err != nil {
			return err
		}
	}

	return nil
}

func (m *MysqlRepManager) CheckCluster() error {
	for _, db := range m.Mysqls {
		if err := db.CheckStatus(); err != nil {
			return err
		}
	}
	return nil
}

func (m *MysqlRepManager) preCompose() error {
	//select master
	masterkey := m.electMaster()
	if err := m.setMysqlType(masterkey, masterRole); err != nil {
		return err
	}

	for _, db := range m.Mysqls {
		if db.GetKey() != masterkey {
			if err := m.setMysqlType(db.GetKey(), slaveRole); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *MysqlRepManager) getMasterKey() (string, bool) {
	for _, db := range m.Mysqls {
		if db.GetType() == masterRole {
			return db.GetKey(), true
		}
	}

	return "", false
}

func (m *MysqlRepManager) electMaster() string {

	curweight := -1
	master := Mysql{}
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

func (m *MysqlRepManager) setMysqlType(dbkey string, typ dbRole) error {

	tmp, ok := m.Mysqls[dbkey]
	if !ok {
		return errors.New("don't find the db key")
	}

	tmp.RoleType = typ
	m.Mysqls[dbkey] = tmp

	return nil

}
