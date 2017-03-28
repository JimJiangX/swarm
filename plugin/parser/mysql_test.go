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
	config config.Configer
}

func (mysqlConfig) clone() parser {
	return &mysqlConfig{}
}

func (mysqlConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *mysqlConfig) Set(key string, val interface{}) error {
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

	m := make(map[string]interface{}, 10)

	for key, val := range desc.Options {
		_ = key
		_ = val
	}

	//	if len(u.networkings) == 1 {
	//		m["mysqld::bind_address"] = u.networkings[0].IP.String()
	//	} else {
	//		return nil, errors.New("unexpected IPAddress")
	//	}

	//	found := false
	//	for i := range u.ports {
	//		if u.ports[i].Name == "mysqld::port" {
	//			m["mysqld::port"] = u.ports[i].Port
	//			m["mysqld::server_id"] = u.ports[i].Port
	//			found = true
	//		}
	//	}
	//	if !found {
	//		return nil, errors.New("unexpected port allocation")
	//	}

	//	m["mysqld::log_bin"] = fmt.Sprintf("/DBAASLOG/BIN/%s-binlog", u.Name)
	//	m["mysqld::innodb_buffer_pool_size"] = int(float64(u.config.HostConfig.Memory) * 0.75)
	//	m["mysqld::relay_log"] = fmt.Sprintf("/DBAASLOG/REL/%s-relay", u.Name)

	//	m["client::user"] = ""
	//	m["client::password"] = ""

	//	users, err := svc.getUsers()
	//	if err != nil {
	//		logrus.WithFields(logrus.Fields{
	//			"Service": svc.Name,
	//			"Unit":    u.Name,
	//		}).Errorf("get Service users,%+v", err)

	//	} else {

	//		for i := range users {
	//			if users[i].Role == _User_DBA_Role {
	//				m["client::user"] = users[i].Username
	//				m["client::password"] = users[i].Password

	//				break
	//			}
	//		}
	//	}

	for key, val := range m {
		err = c.Set(key, val)
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
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, errors.Wrap(err, "create Tempfile")
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	err = c.config.SaveConfigFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(tmpfile.Name())

	return data, errors.Wrap(err, "read file")
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
	//	if c.config == nil || len(args) < 3 {
	//		return healthCheck{}, errors.New("params not ready")
	//	}

	//	addr := c.config.String("mysqld::bind_address")
	//	port, err := c.config.Int("mysqld::port")
	//	if err != nil {
	//		return healthCheck{}, errors.Wrap(err, "get 'mysqld::port'")
	//	}
	//	return healthCheck{
	//		Addr:     addr,
	//		Port:     port,
	//		Script:   "/opt/DBaaS/script/check_db.sh " + args[0] + " " + args[1] + " " + args[2],
	//		Shell:    "",
	//		Interval: "10s",
	//		//TTL:      "15s",
	//		Tags: nil,
	//	}, nil

	return structs.ServiceRegistration{}, nil
}
