package parser

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/vars"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

func init() {
	register("mysql", "5.6", &mysqlConfig{})
	register("mysql", "5.7", &mysqlConfig{})

	register("upsql", "1.0", &upsqlConfig{})
	register("upsql", "1.1", &upsqlConfig{})
	register("upsql", "1.2", &upsqlConfig{})

	register("upsql", "2.0", &upsqlConfig{})
	register("upsql", "2.1", &upsqlConfig{})
	register("upsql", "2.2", &upsqlConfig{})

	register("upsql", "3.0", &upsqlConfig{})
	register("upsql", "3.1", &upsqlConfig{})
	register("upsql", "3.2", &upsqlConfig{})
}

const (
	monitorRole = "monitor"
	rootRole    = "root"
)

const (
	units_prefix    = "units_for_generate_config_"
	units_init_once = "units_init"
	units_add       = "units_add"
	unit_exist      = "unit_exit"
)

var primes = [...]int{11, 13, 17, 19, 23, 29, 31,
	37, 41, 43, 47, 53, 59, 61, 67, 71, 73, 79,
	83, 89, 97, 101, 103, 107, 109, 113, 127, 131,
	137, 139, 149, 151, 157, 163, 167, 173, 179,
	181, 191, 193, 197, 199, 211, 223, 227, 229,
	233, 239, 241, 251, 257, 263, 269, 271, 277,
	281, 283, 293, 307, 311, 313, 317, 331, 337,
	347, 349, 353, 359}

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

func (c mysqlConfig) get(key string) (string, bool) {
	if c.config == nil {
		return "", false
	}

	if val, ok := beegoConfigString(c.config, key); ok {
		return val, ok
	}

	if c.template != nil {
		for i := range c.template.Keysets {
			if c.template.Keysets[i].Key == key {
				return c.template.Keysets[i].Default, false
			}
		}
	}

	return "", false
}

func beegoConfigString(config config.Configer, key string) (string, bool) {
	if val := config.String(key); val != "" {
		return val, true
	}

	section := "default"
	parts := strings.SplitN(key, "::", 2)
	if len(parts) == 2 {
		section = parts[0]
		key = parts[1]
	}

	m, err := config.GetSection(section)
	if err != nil || len(m) == 0 {
		return "", false
	}

	val, ok := m[key]

	return val, ok
}

func (c *mysqlConfig) set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("mysqlConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c *mysqlConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.Validate(desc.Options)
	if err != nil {
		return err
	}

	var (
		found = false
		spec  = structs.UnitSpec{}
		m     = make(map[string]interface{}, 20)
	)

	for i := range desc.Units {
		if id == desc.Units[i].ID {
			spec = desc.Units[i]
			found = true
			// units init once
			m["mysqld::auto_increment_increment"] = i + 1
			break
		}
	}

	if !found {
		return errors.Errorf("not found unit '%s'", id)
	}

	typ, ok := desc.Options[units_prefix+id].(string)
	if ok && typ == unit_exist {
		delete(m, "mysqld::auto_increment_increment")
	} else if !ok {
		// add new unit
		n, ok := desc.Options[units_prefix+id].(int)
		if ok && n > 1 && n <= len(desc.Units) {
			m["mysqld::auto_increment_increment"] = n
		}
	}

	for i, n, max := 0, len(desc.Units), len(primes); i < max; i++ {
		if primes[i] > n {
			m["mysqld::auto_increment_offset"] = primes[i]
			break
		}
	}

	ip := ""
	if len(spec.Networking) >= 1 {
		m["mysqld::bind_address"] = spec.Networking[0].IP
		ip = spec.Networking[0].IP
	} else {
		return errors.New("miss mysqld::bind_address")
	}

	m["mysqld::server_id"] = strconv.Itoa(int(utils.IPToUint32(ip)))

	if v, ok := desc.Options["mysqld::character_set_server"]; ok && v != nil {
		m["mysqld::character_set_server"] = v
	}

	{
		val, ok := desc.Options["mysqld::port"]
		if !ok {
			return errors.New("miss mysqld::port")
		}
		port, err := atoi(val)
		if err != nil || port == 0 {
			return errors.Wrap(err, "miss mysqld::port")
		}

		m["mysqld::port"] = port
	}

	if c.template != nil {
		dat := filepath.Join(c.template.DataMount, "/DAT")
		m["mysqld::tmpdir"] = dat
		m["mysqld::datadir"] = dat

		m["mysqld::socket"] = filepath.Join(c.template.DataMount, "/mysql.sock")

		m["mysqld::log_bin"] = filepath.Clean(fmt.Sprintf("%s/BIN/%s-binlog", c.template.LogMount, spec.Name))

		m["mysqld::relay_log"] = filepath.Clean(fmt.Sprintf("%s/REL/%s-relay", c.template.LogMount, spec.Name))

		m["mysqld::slow_query_log_file"] = filepath.Join(c.template.LogMount, "/slow-query.log")

		m["mysqld::innodb_log_group_home_dir"] = filepath.Join(c.template.LogMount, "/RED")
	}

	if spec.Config != nil {
		if n := spec.Config.HostConfig.Memory; n>>33 > 0 { // 8G
			m["mysqld::innodb_buffer_pool_size"] = int(float64(n) * 0.70)
		} else {
			m["mysqld::innodb_buffer_pool_size"] = int(float64(n) * 0.5)
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

	cmds[structs.InitServiceCmd] = []string{"/root/mysql-init.sh"}

	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

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
	spec, err := getUnitSpec(desc.Units, id)
	if err != nil {
		return structs.ServiceRegistration{}, err
	}

	reg := structs.HorusRegistration{}
	reg.Service.Select = true
	reg.Service.Name = spec.ID
	reg.Service.Type = "unit_" + desc.Image.Name
	reg.Service.Tag = desc.ID
	reg.Service.Container.Name = spec.Name
	reg.Service.Container.HostName = spec.Engine.Node

	reg.Service.MonitorUser = vars.Monitor.User
	reg.Service.MonitorPassword = vars.Monitor.Password

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

	// consul AgentServiceRegistration
	addr := c.config.String("mysqld::bind_address")
	port, err := c.config.Int("mysqld::port")
	if err != nil {
		return structs.ServiceRegistration{}, errors.Wrap(err, "get 'mysqld::port'")
	}

	consul := api.AgentServiceRegistration{
		ID:      spec.Name,
		Name:    spec.Name,
		Tags:    nil,
		Port:    port,
		Address: addr,
		Check: &api.AgentServiceCheck{
			Args: []string{"/usr/local/swarm-agent/scripts/unit_upsql_status.sh", spec.Name},
			// sh -x /usr/local/swarm-agent/scripts/unit_upsql_status.sh 015dc1f7_yeumj001
			// Shell: "/bin/bash",
			// DockerContainerID:           spec.Unit.ContainerID,
			Interval:                       "30s",
			DeregisterCriticalServiceAfter: "30m",
		},
	}

	return structs.ServiceRegistration{
		Horus:  &reg,
		Consul: &consul}, nil
}

type upsqlConfig struct {
	mysqlConfig
}

func (upsqlConfig) clone(t *structs.ConfigTemplate) parser {
	pr := &upsqlConfig{}
	pr.template = t

	return pr
}

func (upsqlConfig) GenerateCommands(id string, desc structs.ServiceSpec) (structs.CmdsMap, error) {
	cmds := make(structs.CmdsMap, 7)

	cmds[structs.StartContainerCmd] = []string{"/bin/bash"}

	cmds[structs.InitServiceCmd] = []string{"/root/upsql-init.sh"}

	cmds[structs.StartServiceCmd] = []string{"/root/serv", "start"}

	cmds[structs.StopServiceCmd] = []string{"/root/serv", "stop"}

	cmds[structs.RestoreCmd] = []string{"/root/upsql-restore.sh"}

	cmds[structs.BackupCmd] = []string{"/root/upsql-backup.sh"}

	cmds[structs.MigrateRebuildCmd] = []string{"/root/upsql-config-init.sh"}

	return cmds, nil
}

func (c *upsqlConfig) GenerateConfig(id string, desc structs.ServiceSpec) error {
	err := c.mysqlConfig.GenerateConfig(id, desc)
	if err != nil {
		return err
	}

	if c.template != nil {
		err = c.set("mysqld::socket", filepath.Join(c.template.DataMount, "/upsql.sock"))
	}

	return errors.WithStack(err)
}

func getUnitSpec(units []structs.UnitSpec, id string) (structs.UnitSpec, error) {
	for i := range units {
		if id == units[i].ID {
			return units[i], nil
		}
	}

	return structs.UnitSpec{}, errors.Errorf("not found unit '%s'", id)
}
