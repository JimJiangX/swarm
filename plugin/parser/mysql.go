package parser

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

func init() {
	register("mysql", "5.6", &mysqlConfig{})
	register("mysql", "5.7", &mysqlConfig{})
}

type mysqlConfig struct {
	template *structs.ConfigTemplate
	config   config.Configer
}

func (mysqlConfig) clone(t *structs.ConfigTemplate) parser {
	return &mysqlConfig{
		template: t,
	}
}

func (mysqlConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *mysqlConfig) set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("mysqlConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c mysqlConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	var spec *structs.UnitSpec

	for i := range desc.Units {
		if id == desc.Units[i].ID {
			spec = &desc.Units[i]
			break
		}
	}

	if spec == nil {
		return errors.Errorf("not found unit '%s' in service '%s'", id, desc.Name)
	}

	m := make(map[string]interface{}, 20)

	if len(spec.Networking) >= 1 {
		m["mysqld::bind_address"] = spec.Networking[0].IP
	} else {
		return errors.New("unexpected IPAddress")
	}

	m["mysqld::port"] = desc.Options["port"]
	m["mysqld::server_id"] = desc.Options["port"]

	if c.template != nil {
		m["mysqld::log_bin"] = fmt.Sprintf("%s/BIN/%s-binlog", c.template.LogMount, spec.Name)

		m["mysqld::relay_log"] = fmt.Sprintf("%s/REL/%s-relay", c.template.LogMount, spec.Name)
	}

	if n := spec.Config.HostConfig.Memory; n>>33 > 0 {
		m["mysqld::innodb_buffer_pool_size"] = int(float64(n) * 0.70)
	} else {
		m["mysqld::innodb_buffer_pool_size"] = int(float64(n) * 0.5)
	}

	m["client::user"] = ""
	m["client::password"] = ""

	var dba *structs.User

	if len(desc.Users) > 0 {
		for i := range desc.Users {
			if desc.Users[i].Role == "dba" {
				dba = &desc.Users[i]
				break
			}
		}

		if dba != nil {
			m["client::user"] = dba.Name
			m["client::password"] = dba.Password
		}
	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

func (c mysqlConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 6)

	cmds[structs.StartContainerCmd] = []string{"/bin/bash"}

	//func (mysqlCmd) InitServiceCmd(args ...string) []string {
	//	cmd := make([]string, len(args)+1)
	//	cmd[0] = "/root/upsql-init.sh"
	//	copy(cmd[1:], args)
	//	return cmd
	//}
	cmds[structs.InitServiceCmd] = []string{"/root/upsql-init.sh"}

	cmds[structs.StartServiceCmd] = []string{"/root/upsql.service", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/upsql.service", "stop"}

	//func (mysqlCmd) RestoreCmd(file, backupDir string) []string {
	//	return []string{"/root/upsql-restore.sh", file, backupDir}
	//}
	cmds[structs.RestoreCmd] = []string{"/root/upsql-restore.sh"}

	//func (mysqlCmd) BackupCmd(args ...string) []string {
	//	cmd := make([]string, len(args)+1)
	//	cmd[0] = "/root/upsql-backup.sh"
	//	copy(cmd[1:], args)
	//	return cmd
	//}
	cmds[structs.BackupCmd] = []string{"/root/upsql-backup.sh"}

	return cmds, nil
}

func (c *mysqlConfig) ParseData(data []byte) error {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return errors.Wrap(err, "parse ini file")
	}

	c.config = configer

	return nil
}

func (c mysqlConfig) Marshal() ([]byte, error) {
	file, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, errors.Wrap(err, "create Tempfile")
	}
	file.Close()
	defer os.Remove(file.Name())

	err = c.config.SaveConfigFile(file.Name())
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(file.Name())
	if err == nil {
		return data, nil
	}

	return data, errors.WithStack(err)
}

//func (mysqlConfig) Requirement() structs.RequireResource {
//	ports := []port{
//		port{
//			proto: "tcp",
//			name:  "mysqld::port",
//		},
//	}
//	nets := []netRequire{
//		netRequire{
//			Type: _ContainersNetworking,
//			num:  1,
//		},
//	}
//	return require{
//		ports:       ports,
//		networkings: nets,
//	}

//	return structs.RequireResource{}
//}

func (c mysqlConfig) HealthCheck(id string, desc structs.ServiceSpec) (structs.ServiceRegistration, error) {
	var spec *structs.UnitSpec

	for i := range desc.Units {
		if id == desc.Units[i].ID {
			spec = &desc.Units[i]
			break
		}
	}

	if spec == nil {
		return structs.ServiceRegistration{}, errors.Errorf("not found unit '%s' in service '%s'", id, desc.Name)
	}

	im, err := structs.ParseImage(c.template.Image)
	if err != nil {
		return structs.ServiceRegistration{}, err
	}

	reg := structs.HorusRegistration{}
	reg.Service.Select = true
	reg.Service.Name = spec.ID
	reg.Service.Type = "unit_" + im.Name
	reg.Service.Tag = desc.ID
	reg.Service.Container.Name = spec.Container.ID
	reg.Service.Container.HostName = spec.Engine.Node

	var mon *structs.User

	if len(desc.Users) > 0 {
		for i := range desc.Users {
			if desc.Users[i].Role == "mon" {
				mon = &desc.Users[i]
				break
			}
		}

		if mon != nil {
			reg.Service.MonitorUser = mon.Name
			reg.Service.MonitorPassword = mon.Password
		}
	}

	return structs.ServiceRegistration{Horus: &reg}, nil
}
