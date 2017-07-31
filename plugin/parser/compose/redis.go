package compose

import (
	"strconv"
	"time"

	"github.com/pkg/errors"
)

type Redis struct {
	Ip   string
	Port int

	scriptDir string

	Weight   int //Weight越高，优先变成master，等值随机
	RoleType dbRole
}

func (r Redis) GetKey() string {
	return r.Ip + ":" + strconv.Itoa(r.Port)
}

func (m Redis) Clear() error {
	filepath := m.scriptDir + ""
	timeout := time.Second * 60
	args := []string{}

	_, err := ExecShellFileTimeout(filepath, timeout, args...)

	return err
}

func (m Redis) GetType() dbRole {
	return m.RoleType
}

func (m Redis) ChangeMaster(master Redis) error {
	if m.GetType() != masterRole && m.GetType() != slaveRole {
		return errors.New(string(m.GetType()) + ":should not call the func")
	}

	// TODO:script path
	filepath := m.scriptDir + ""
	timeout := time.Second * 60
	args := []string{}

	_, err := ExecShellFileTimeout(filepath, timeout, args...)

	return err
}

func (m Redis) CheckStatus() error {
	filepath := m.scriptDir + ""
	timeout := time.Second * 60
	args := []string{}

	_, err := ExecShellFileTimeout(filepath, timeout, args...)

	return err
}
