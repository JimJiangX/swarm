package compose

import (
	"errors"
	"strconv"
	"time"
)

type ROLE_TYPE string

const (
	MGROUP_TYPE ROLE_TYPE = "MGROUP"
	MASTER_TYPE ROLE_TYPE = "MASTER"
	SLAVE_TYPE  ROLE_TYPE = "SLAVE"

//	NONE_STATUS MYSQL_STATUS = "WAITING_CHECK"
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
	RoleType ROLE_TYPE

	MgmPort int
	MgmIP   string
}

func (m Mysql) GetKey() string {
	return m.IP + ":" + strconv.Itoa(m.Port)
}

func (m Mysql) Clear() error {
	filepath := BASEDIR + "mysql-replication-reset.sh"
	timeout := time.Second * 60
	args := []string{
		m.Instance,
		m.MgmIP,
		strconv.Itoa(m.MgmPort),
	}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}

func (m Mysql) GetType() ROLE_TYPE {
	return m.RoleType
}

func (m Mysql) ChangeMaster(master Mysql) error {
	if m.GetType() != MASTER_TYPE && m.GetType() != SLAVE_TYPE {
		return errors.New(string(m.GetType()) + ":should not call the func")
	}

	filepath := BASEDIR + "mysql-replication-set.sh"
	timeout := time.Second * 60

	args := []string{
		m.Instance,
		m.MgmIP,
		strconv.Itoa(m.MgmPort),
		string(m.RoleType),
		master.IP,
		strconv.Itoa(master.MgmPort),
		m.ReplicateUser,
		m.Replicatepwd,
		m.IP,
		strconv.Itoa(m.Port),
	}

	_, err := ExecShellFileTimeout(filepath, timeout, args...)

	return err
}

func (m Mysql) CheckStatus() error {
	filepath := BASEDIR + "mysqlcheck.sh"
	timeout := time.Second * 60
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}
