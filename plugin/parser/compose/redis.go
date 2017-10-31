package compose

import (
	"strconv"
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

func (m Redis) GetType() dbRole {
	return m.RoleType
}

//func (m Redis) Clear() error {
//	args := []string{m.scriptDir + ""}

//	out, err := utils.ExecContextTimeout(nil, defaultTimeout, args...)

//	logrus.Debugf("exec:%s,output:%s", args, out)

//	return err
//}

//func (m Redis) ChangeMaster(master Redis) error {
//	if m.GetType() != masterRole && m.GetType() != slaveRole {
//		return errors.New(string(m.GetType()) + ":should not call the func")
//	}

//	// TODO:script path
//	args := []string{m.scriptDir + ""}

//	out, err := utils.ExecContextTimeout(nil, defaultTimeout, args...)

//	logrus.Debugf("exec:%s,output:%s", args, out)

//	return err
//}

//func (m Redis) CheckStatus() error {
//	args := []string{m.scriptDir + ""}

//	out, err := utils.ExecContextTimeout(nil, defaultTimeout, args...)

//	logrus.Debugf("exec:%s,output:%s", args, out)

//	return err
//}
