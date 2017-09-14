package parser

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

func init() {
	register("mysql", "5.6", &mysqlConfig{})
	register("mysql", "5.7", &mysqlConfig{})

	register("upsql", "1.0", &mysqlConfig{})
	register("upsql", "2.0", &mysqlConfig{})
	register("upsql", "3.0", &mysqlConfig{})
}

const (
	monitorRole = "monitor"
	rootRole    = "root"
)

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

func (c mysqlConfig) get(key string) string {
	if c.config == nil {
		return ""
	}

	if val := c.config.String(key); val != "" {
		return val
	}

	if c.template != nil {
		for i := range c.template.Keysets {
			if c.template.Keysets[i].Key == key {
				return c.template.Keysets[i].Default
			}
		}
	}

	return ""
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

	m["mysqld::server_id"] = net.ParseIP(spec.Networking[0].IP).To4()[3]

	if v, ok := desc.Options["character_set_server"]; ok && v != nil {
		m["mysqld::character_set_server"] = v
	}

	if p, ok := desc.Options["port"]; ok && p != nil {
		m["mysqld::port"] = p
	}

	if c.template != nil {
		m["mysqld::log_bin"] = filepath.Clean(fmt.Sprintf("%s/BIN/%s-binlog", c.template.LogMount, spec.Name))

		m["mysqld::relay_log"] = filepath.Clean(fmt.Sprintf("%s/REL/%s-relay", c.template.LogMount, spec.Name))
	}

	if spec.Config != nil {
		if n := spec.Config.HostConfig.Memory; n>>33 > 0 {
			m["mysqld::innodb_buffer_pool_size"] = int(float64(n) * 0.70)
		} else {
			m["mysqld::innodb_buffer_pool_size"] = int(float64(n) * 0.5)
		}
	}

	m["client::user"] = ""
	m["client::password"] = ""

	var root *structs.User

	if len(desc.Users) > 0 {
		for i := range desc.Users {
			if desc.Users[i].Role == rootRole {
				root = &desc.Users[i]
				break
			}
		}

		if root != nil {
			m["client::user"] = root.Name
			m["client::password"] = root.Password
		}
	}

	for key, val := range m {
		err = c.set(key, val)
	}

	return err
}

func (c mysqlConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 6)

	//	users := make([]string, 0, len(desc.Users)*4)
	//	for _, u := range desc.Users {
	//		users = append(users, u.Role, u.Name, u.Password, u.Privilege)
	//	}
	//	init := make([]string, 1+len(users))
	//	init[0] = "/root/mysql-init.sh"
	//	copy(init[1:], users)

	cmds[structs.StartContainerCmd] = []string{"/bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/mysql-init.sh"}

	cmds[structs.StartServiceCmd] = []string{"/root/mysql.service", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/mysql.service", "stop"}

	cmds[structs.RestoreCmd] = []string{"/root/mysql-restore.sh"}

	cmds[structs.BackupCmd] = []string{"/root/mysql-backup.sh"}

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
	reg.Service.Container.Name = spec.Name
	reg.Service.Container.HostName = spec.Engine.Node

	var mon *structs.User

	if len(desc.Users) > 0 {
		for i := range desc.Users {
			if desc.Users[i].Role == monitorRole {
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
