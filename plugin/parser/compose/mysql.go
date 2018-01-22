package compose

import (
	"path/filepath"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

const defaultTimeout = time.Minute

type mysqlUser struct {
	user     string
	password string

	//	Rootuser string
	//	RootPwd  string
}

type Mysql struct {
	IP       string
	Port     int
	Instance string

	user mysqlUser

	Weight   int //Weight越高，优先变成master，等值随机
	RoleType dbRole

	MgmPort int
	MgmIP   string

	scriptDir string
}

func (m Mysql) GetKey() string {
	return m.IP + ":" + strconv.Itoa(m.Port)
}

func (m Mysql) Clear() error {
	args := []string{
		filepath.Join(m.scriptDir, "mysql-replication-reset.sh"),
		m.Instance,
		m.MgmIP,
		strconv.Itoa(m.MgmPort),
	}

	out, err := utils.ExecContextTimeout(nil, defaultTimeout, args...)
	logrus.Debugf("exec:%s,output:%s", args, out)

	return err
}

func (m Mysql) GetType() dbRole {
	return m.RoleType
}

func (m Mysql) ChangeMaster(master Mysql) error {
	if m.GetType() != masterRole && m.GetType() != slaveRole {
		return errors.New(string(m.GetType()) + ":should not call the func")
	}

	args := []string{
		filepath.Join(m.scriptDir, "mysql-replication-set.sh"),
		m.Instance,
		m.MgmIP,
		strconv.Itoa(m.MgmPort),
		string(m.RoleType),
		master.IP,
		strconv.Itoa(master.Port),
		m.user.user,
		m.user.password,
		m.IP,
		strconv.Itoa(m.Port),
	}

	out, err := utils.ExecContextTimeout(nil, defaultTimeout, args...)

	logrus.Debugf("exec:%s,output:%s", args, out)

	return err
}

func (m Mysql) CheckStatus() error {
	args := filepath.Join(m.scriptDir, "mysqlcheck.sh")
	out, err := utils.ExecContextTimeout(nil, defaultTimeout, args)

	logrus.Debugf("exec:%s,output:%s", args, out)

	return err
}
