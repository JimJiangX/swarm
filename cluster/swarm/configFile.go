package swarm

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

func (u unit) Path() string {
	if u.parent == nil {
		return ""
	}

	return u.parent.Path
}

func (u unit) CanModify(data map[string]interface{}) ([]string, bool) {
	if len(u.parent.KeySets) == 0 {
		return nil, true
	}

	can := true
	keys := make([]string, 0, len(u.parent.KeySets))

	for key := range data {
		if !u.parent.KeySets[key] {
			keys = append(keys, key)
			can = false
		}
	}

	return keys, can
}

func (u unit) Verify(data map[string]interface{}) error {
	if len(data) > 0 {
		if err := u.Validate(data); err != nil {
			return err
		}
	}

	return nil
}

func (u *unit) Set(key string, val interface{}) error {
	if !u.parent.KeySets[key] {
		return fmt.Errorf("%s cannot Set new Value", key)
	}

	return u.set(key, val)
}

func (u *unit) set(key string, val interface{}) error {

	return nil
}

func (u *unit) SaveConfigToDisk(content []byte) (_ string, err error) {

	config := database.UnitConfig{
		ID:        utils.Generate64UUID(),
		ImageID:   u.ImageID,
		Version:   u.parent.Version + 1,
		ParentID:  u.parent.ID,
		Content:   string(content),
		KeySets:   u.parent.KeySets,
		Path:      u.Path(),
		CreatedAt: time.Now(),
	}

	u.Unit.ConfigID = config.ID

	err = database.SaveUnitConfigToDisk(&u.Unit, config)
	if err != nil {
		return "", err
	}

	return config.ID, nil
}

type mysqlCmd struct{}

func (mysqlCmd) StartContainerCmd() []string     { return nil }
func (mysqlCmd) StartServiceCmd() []string       { return nil }
func (mysqlCmd) StopServiceCmd() []string        { return nil }
func (mysqlCmd) RecoverCmd(file string) []string { return nil }
func (mysqlCmd) BackupCmd(args ...string) []string {
	cmd := make([]string, len(args)+1)
	cmd[0] = "/root/upsql-backup.sh"
	copy(cmd[1:], args)

	return cmd
}

func (mysqlCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type mysqlConfig struct {
	config config.Configer
	port   port
}

func (mysqlConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c mysqlConfig) defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error) {
	m := make(map[string]interface{}, 10)
	if len(u.networkings) == 1 {
		m["mysqld::bind-address"] = u.networkings[0].IP.String()
	} else {
		return nil, fmt.Errorf("Unexpected,more than 1 IPAddress allocated")
	}

	if len(u.ports) == 1 {
		m["mysqld::port"] = u.ports[0].Port
		m["mysqld::server-id"] = u.ports[0].Port
	} else {
		return nil, fmt.Errorf("Unexpected,more than 1 port allocated")
	}

	m["mysqld::character_set_server"] = "gbk"
	m["mysqld::log-bin"] = fmt.Sprintf("/DBAASLOG/BIN/%s-binlog", u.Name)
	m["mysqld::innodb_buffer_pool_size"] = int(float64(u.config.HostConfig.Memory) * 0.75)
	m["mysqld::relay_log"] = fmt.Sprintf("/DBAASLOG/REL/%s-relay", u.Name)

	return nil, nil
}

func (c *mysqlConfig) ParseData(data []byte) (config.Configer, error) {
	// ini/json/xml
	// convert to map[string]interface{}

	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return nil, err
	}

	c.config = configer

	return c.config, nil
}

func (c mysqlConfig) Marshal() ([]byte, error) {
	// convert to ini/json/xml

	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	err = c.config.SaveConfigFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(tmpfile)
}

type port struct {
	port  int
	proto string
	name  string
}

func (c mysqlConfig) PortSlice() (bool, []port) {
	if c.port != (port{}) {
		return true, []port{c.port}
	}
	return false, []port{port{proto: "tcp", name: ""}}
}

type proxyCmd struct{}

func (proxyCmd) StartContainerCmd() []string                { return nil }
func (proxyCmd) StartServiceCmd() []string                  { return nil }
func (proxyCmd) StopServiceCmd() []string                   { return nil }
func (proxyCmd) RecoverCmd(file string) []string            { return nil }
func (proxyCmd) BackupCmd(args ...string) []string          { return nil }
func (proxyCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type proxyConfig struct {
	ports []port
}

func (proxyConfig) Validate(data map[string]interface{}) error     { return nil }
func (proxyConfig) ParseData(data []byte) (config.Configer, error) { return nil, nil }
func (proxyConfig) Marshal() ([]byte, error)                       { return nil, nil }
func (c proxyConfig) PortSlice() (bool, []port) {

	return false, []port{
		port{proto: "tcp", name: "proxy_data_port"},
		port{proto: "tcp", name: "proxy_admin_port"},
	}
}
func (c proxyConfig) defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error) {
	m := make(map[string]interface{}, 10)
	m["upsql-proxy::proxy-domain"] = svc.ID
	m["upsql-proxy::proxy-name"] = u.ID
	if len(u.networkings) == 2 && len(u.ports) == 2 {
		adminAddr, dataAddr := "", ""
		adminPort, dataPort := 0, 0
		for i := range u.networkings {
			if u.networkings[i].Type == _ContainersNetworking {
				adminAddr = u.networkings[i].IP.String()
			} else if u.networkings[i].Type == _ExternalAccessNetworking {
				dataAddr = u.networkings[i].IP.String()
			}
		}

		for i := range u.ports {
			if u.ports[i].Name == "proxy_data_port" {
				dataPort = u.ports[i].Port
			} else if u.ports[i].Name == "proxy_admin_port" {
				adminPort = u.ports[i].Port
			}
		}
		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", dataAddr, dataPort)
		m["adm-cli::adm-cli-address"] = fmt.Sprintf("%s:%d", adminAddr, adminPort)
	}

	m["upsql-proxy::event-threads-count"] = u.config.HostConfig.CpusetCpus

	return m, nil
}

type switchManagerCmd struct{}

func (switchManagerCmd) StartContainerCmd() []string                { return nil }
func (switchManagerCmd) StartServiceCmd() []string                  { return nil }
func (switchManagerCmd) StopServiceCmd() []string                   { return nil }
func (switchManagerCmd) RecoverCmd(file string) []string            { return nil }
func (switchManagerCmd) BackupCmd(args ...string) []string          { return nil }
func (switchManagerCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type switchManagerConfig struct {
	ports []port
}

func (switchManagerConfig) Validate(data map[string]interface{}) error     { return nil }
func (switchManagerConfig) ParseData(data []byte) (config.Configer, error) { return nil, nil }
func (switchManagerConfig) Marshal() ([]byte, error)                       { return nil, nil }
func (c switchManagerConfig) PortSlice() (bool, []port) {
	if c.ports != nil {
		return true, c.ports
	}
	return false, []port{port{proto: "tcp", name: "Port"}, port{proto: "tcp", name: "ProxyPort"}}
}
func (c switchManagerConfig) defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error) {
	sys, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	m := make(map[string]interface{}, 10)
	m["domain"] = svc.ID
	m["name"] = u.ID
	port, proxyPort := 0, 0
	for i := range u.ports {
		if u.ports[i].Name == "Port" {
			port = u.ports[i].Port
		} else if u.ports[i].Name == "ProxyPort" {
			proxyPort = u.ports[i].Port
		}
	}
	m["ProxyPort"] = proxyPort
	m["Port"] = port

	// consul
	m["ConsulBindNetworkName"] = u.engine.Labels[_Admin_NIC_Lable]
	m["SwarmHostKey"] = leaderElectionPath
	m["ConsulPort"] = sys.ConsulPort

	return nil, nil
}
