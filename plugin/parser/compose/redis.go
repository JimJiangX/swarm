package compose

import (
	"errors"

	"strconv"
	"time"
	//	log "github.com/Sirupsen/logrus"
)

type Redis struct {
	Ip   string
	Port int

	Weight   int //Weight越高，优先变成master，等值随机
	RoleType ROLE_TYPE
}

func (r Redis) GetKey() string {
	return r.Ip + ":" + strconv.Itoa(r.Port)
}

func (m Redis) Clear() error {
	filepath := BASEDIR + ""
	timeout := time.Second * 60
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}

func (m Redis) GetType() ROLE_TYPE {
	return m.RoleType
}

func (m Redis) ChangeMaster(master Redis) error {
	if m.GetType() != MASTER_TYPE && m.GetType() != SLAVE_TYPE {
		return errors.New(string(m.GetType()) + ":should not call the func")
	}

	filepath := BASEDIR + ""
	timeout := time.Second * 60
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)

	return err
}

func (m Redis) CheckStatus() error {
	filepath := BASEDIR + ""
	timeout := time.Second * 60
	args := []string{}
	_, err := ExecShellFileTimeout(filepath, timeout, args...)
	return err
}
