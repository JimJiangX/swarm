package compose

type Composer interface {
	ClearCluster() error
	CheckCluster() error
	ComposeCluster() error
}

func NewMysqlComposer(arch string, dbs []Mysql, mgmIp string, mgmPort int) Composer {
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

		return ms
	} /*else if arch == "MG" {

	}*/

	return nil

}
