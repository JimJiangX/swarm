package compose

import (
	"errors"
	"strconv"
	"time"
)

type MysqlUser struct {
	ReplicateUser string
	Replicatepwd  string

	//	Rootuser string
	//	RootPwd  string
}

type Mysql struct {
	IP       string
	Port     int
	Instance string

	MysqlUser

	Weight   int //Weight越高，优先变成master，等值随机
	RoleType dbRole

	MgmPort int
	MgmIP   string
}

func (m Mysql) GetKey() string {
	return m.IP + ":" + strconv.Itoa(m.Port)
}

func (m Mysql) Clear() error {
	filepath := scriptDir + "mysql-replication-reset.sh"
	timeout := time.Second * 60
	args := []string{
		m.Instance,
		m.MgmIP,
		strconv.Itoa(m.MgmPort),
	}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}

func (m Mysql) GetType() dbRole {
	return m.RoleType
}

func (m Mysql) ChangeMaster(master Mysql) error {
	if m.GetType() != masterRole && m.GetType() != slaveRole {
		return errors.New(string(m.GetType()) + ":should not call the func")
	}

	filepath := scriptDir + "mysql-replication-set.sh"
	timeout := time.Second * 60

	args := []string{
		m.Instance,
		m.MgmIP,
		strconv.Itoa(m.MgmPort),
		string(m.RoleType),
		master.IP,
		strconv.Itoa(master.Port),
		m.ReplicateUser,
		m.Replicatepwd,
		m.IP,
		strconv.Itoa(m.Port),
	}

	_, err := ExecShellFileTimeout(filepath, timeout, args...)

	return err
}

func (m Mysql) CheckStatus() error {
	filepath := scriptDir + "mysqlcheck.sh"
	timeout := time.Second * 60
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}
