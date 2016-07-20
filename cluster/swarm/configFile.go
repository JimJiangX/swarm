package swarm

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

func (u unit) Path() string {
	if u.parent == nil {
		return "/"
	}

	return u.parent.Mount
}

func (u unit) CanModify(data map[string]interface{}) ([]string, bool) {
	if len(u.parent.KeySets) == 0 {
		return nil, true
	}

	can := true
	keys := make([]string, 0, len(u.parent.KeySets))

	for key := range data {
		// case sensitive
		if !u.parent.KeySets[strings.ToLower(key)].CanSet {
			keys = append(keys, key)
			can = false
		}
	}

	return keys, can
}

func (u unit) MustRestart(data map[string]interface{}) bool {
	for key := range data {
		// case sensitive
		if u.parent.KeySets[strings.ToLower(key)].MustRestart {
			return true
		}
	}

	return false
}

func (u unit) Verify(data map[string]interface{}) error {
	if len(data) > 0 {
		if err := u.Validate(data); err != nil {
			return err
		}
	}

	return nil
}

func (u *unit) SetServiceConfig(key string, val interface{}) (bool, error) {
	// case sensitive
	restart := false
	key = strings.ToLower(key)
	if !u.parent.KeySets[key].CanSet {
		return restart, fmt.Errorf("Key %s cannot Set new Value", key)
	}

	if u.parent.KeySets[key].MustRestart {
		restart = true
	}

	return restart, u.setServiceConfig(key, val)
}

func (u *unit) setServiceConfig(key string, val interface{}) error {
	if u.configParser == nil {
		return fmt.Errorf("Unit %s configParser is nil", u.Name)
	}

	return u.configParser.Set(key, val)
}

func (u *unit) SaveConfigToDisk(content []byte) error {
	config := database.UnitConfig{
		ID:        utils.Generate64UUID(),
		ImageID:   u.ImageID,
		Version:   u.parent.Version + 1,
		ParentID:  u.parent.ID,
		Content:   string(content),
		KeySets:   u.parent.KeySets,
		Mount:     u.Path(),
		CreatedAt: time.Now(),
	}

	u.Unit.ConfigID = config.ID
	u.parent = &config

	err := database.SaveUnitConfigToDisk(&u.Unit, config)

	return err
}

func Factory(_type string) (configParser, ContainerCmd, error) {
	return initialize(_type)
}

func initialize(_type string) (configParser, ContainerCmd, error) {
	var (
		parser configParser
		cmder  ContainerCmd
	)
	switch _type {
	case _UpsqlType:
		parser = &mysqlConfig{}

		cmder = &mysqlCmd{}

	case _ProxyType, "upproxy":
		parser = &proxyConfig{}

		cmder = &proxyCmd{}

	case _SwitchManagerType, "swm":
		parser = &switchManagerConfig{}

		cmder = &switchManagerCmd{}

	default:

		return nil, nil, fmt.Errorf("Unsupported Type:'%s'", _type)
	}

	return parser, cmder, nil
}

type mysqlCmd struct{}

func (mysqlCmd) StartContainerCmd() []string {
	return []string{"/bin/bash"}
}
func (mysqlCmd) InitServiceCmd() []string {
	return []string{"/root/upsql-init.sh"}
}
func (mysqlCmd) StartServiceCmd() []string {
	return []string{"/root/upsql.service", "start"}
}
func (mysqlCmd) StopServiceCmd() []string {
	return []string{"/root/upsql.service", "stop"}
}
func (mysqlCmd) RestoreCmd(file string) []string {
	return []string{"/root/upsql-restore.sh", file}
}
func (mysqlCmd) BackupCmd(args ...string) []string {
	cmd := make([]string, len(args)+1)
	cmd[0] = "/root/upsql-backup.sh"
	copy(cmd[1:], args)

	return cmd
}

func (mysqlCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type mysqlConfig struct {
	config config.Configer
}

func (mysqlConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c mysqlConfig) defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error) {
	found := false
	m := make(map[string]interface{}, 10)
	if svc == nil || u == nil {
		return m, fmt.Errorf("params maybe nil")
	}

	if len(u.networkings) == 1 {
		m["mysqld::bind-address"] = u.networkings[0].IP.String()
	} else {
		return nil, fmt.Errorf("Unexpected IPAddress allocated")
	}

	found = false
	for i := range u.ports {
		if u.ports[i].Name == "mysqld::port" {
			m["mysqld::port"] = u.ports[i].Port
			m["mysqld::server-id"] = u.ports[i].Port
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("Unexpected port allocation")
	}

	m["mysqld::log-bin"] = fmt.Sprintf("/DBAASLOG/BIN/%s-binlog", u.Name)
	m["mysqld::innodb_buffer_pool_size"] = int(float64(u.config.HostConfig.Memory) * 0.75)
	m["mysqld::relay_log"] = fmt.Sprintf("/DBAASLOG/REL/%s-relay", u.Name)

	return m, nil
}

func (c *mysqlConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		return fmt.Errorf("mysqlConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c *mysqlConfig) ParseData(data []byte) (config.Configer, error) {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return nil, err
	}

	c.config = configer

	return c.config, nil
}

func (c mysqlConfig) Marshal() ([]byte, error) {
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, err
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	err = c.config.SaveConfigFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	return ioutil.ReadFile(tmpfile.Name())
}

func (mysqlConfig) Requirement() require {
	ports := []port{
		port{
			proto: "tcp",
			name:  "mysqld::port",
		},
	}
	nets := []netRequire{
		netRequire{
			Type: _ContainersNetworking,
			num:  1,
		},
	}
	return require{
		ports:       ports,
		networkings: nets,
	}
}

type healthCheck struct {
	Port     int
	Script   string
	Shell    string
	Interval string
	Timeout  string
	TTL      string
	Tags     []string
}

func (c mysqlConfig) HealthCheck() (healthCheck, error) {
	if c.config == nil {
		return healthCheck{}, fmt.Errorf("params not ready")
	}

	port, err := c.config.Int("mysqld::port")
	if err != nil {
		return healthCheck{}, err
	}
	return healthCheck{
		Port:     port,
		Script:   "/opt/DBaaS/script/check_db.sh ",
		Shell:    "",
		Interval: "10s",
		//TTL:      "15s",
		Tags: nil,
	}, nil
}

type proxyCmd struct{}

func (proxyCmd) StartContainerCmd() []string {
	return []string{"/bin/bash"}
}
func (proxyCmd) InitServiceCmd() []string {
	return []string{"/root/upproxy.service", "start"}
}
func (proxyCmd) StartServiceCmd() []string {
	return []string{"/root/upproxy.service", "start"}
}
func (proxyCmd) StopServiceCmd() []string {
	return []string{"/root/upproxy.service", "stop"}
}
func (proxyCmd) RestoreCmd(file string) []string            { return nil }
func (proxyCmd) BackupCmd(args ...string) []string          { return nil }
func (proxyCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type proxyConfig struct {
	config config.Configer
}

func (c *proxyConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		return fmt.Errorf("mysqlConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (proxyConfig) Validate(data map[string]interface{}) error { return nil }
func (c *proxyConfig) ParseData(data []byte) (config.Configer, error) {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return nil, err
	}

	c.config = configer

	return c.config, nil
}

func (c *proxyConfig) Marshal() ([]byte, error) {
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, err
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	err = c.config.SaveConfigFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	return ioutil.ReadFile(tmpfile.Name())
}

func (proxyConfig) Requirement() require {
	ports := []port{
		port{
			proto: "tcp",
			name:  "proxy_data_port",
		},
		port{
			proto: "tcp",
			name:  "proxy_admin_port",
		},
	}
	nets := []netRequire{
		netRequire{
			Type: _ContainersNetworking,
			num:  1,
		},
		netRequire{
			Type: _ExternalAccessNetworking,
			num:  1,
		},
	}
	return require{
		ports:       ports,
		networkings: nets,
	}
}

func (c proxyConfig) HealthCheck() (healthCheck, error) {
	if c.config == nil {
		return healthCheck{}, fmt.Errorf("params not ready")
	}

	port, err := c.config.Int("adm-cli::proxy_admin_port")
	if err != nil {
		return healthCheck{}, err
	}
	return healthCheck{
		Port:     port,
		Script:   "/opt/DBaaS/script/check_proxy.sh ",
		Shell:    "",
		Interval: "10s",
		TTL:      "15s",
		Tags:     nil,
	}, nil
}

func (c proxyConfig) defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error) {
	m := make(map[string]interface{}, 10)
	m["upsql-proxy::proxy-domain"] = svc.ID
	m["upsql-proxy::proxy-name"] = u.Name
	if len(u.networkings) == 2 && len(u.ports) >= 2 {
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
				m["adm-cli::proxy_admin_port"] = adminPort
			}
		}
		m["upsql-proxy::proxy-address"] = fmt.Sprintf("%s:%d", dataAddr, dataPort)
		m["adm-cli::adm-cli-address"] = fmt.Sprintf("%s:%d", adminAddr, adminPort)
	}

	m["upsql-proxy::event-threads-count"] = u.config.HostConfig.CpusetCpus

	swm := svc.getSwithManagerUnit()
	if swm != nil {
		swmProxyPort := 0
		for i := range swm.ports {
			if swm.ports[i].Name == "ProxyPort" {
				swmProxyPort = swm.ports[i].Port
				break
			}
		}
		if len(swm.networkings) == 1 {
			m["adm-cli::adm-svr-address"] = fmt.Sprintf("%s:%d", swm.networkings[0].IP.String(), swmProxyPort)
		}
	}

	return m, nil
}

type switchManagerCmd struct{}

func (switchManagerCmd) StartContainerCmd() []string {
	return []string{"/bin/bash"}
}
func (switchManagerCmd) InitServiceCmd() []string {
	return []string{"/root/swm.service", "start"}
}
func (switchManagerCmd) StartServiceCmd() []string {
	return []string{"/root/swm.service", "start"}
}
func (switchManagerCmd) StopServiceCmd() []string {
	return []string{"/root/swm.service", "stop"}
}
func (switchManagerCmd) RestoreCmd(file string) []string            { return nil }
func (switchManagerCmd) BackupCmd(args ...string) []string          { return nil }
func (switchManagerCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type switchManagerConfig struct {
	config config.Configer
}

func (c *switchManagerConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		return fmt.Errorf("switchManagerConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (switchManagerConfig) Validate(data map[string]interface{}) error { return nil }
func (c *switchManagerConfig) ParseData(data []byte) (config.Configer, error) {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return nil, err
	}

	c.config = configer

	return c.config, nil
}

func (c *switchManagerConfig) Marshal() ([]byte, error) {
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, err
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	err = c.config.SaveConfigFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	return ioutil.ReadFile(tmpfile.Name())
}

func (switchManagerConfig) Requirement() require {
	ports := []port{
		port{
			proto: "tcp",
			name:  "Port",
		},
		port{
			proto: "tcp",
			name:  "ProxyPort",
		},
	}
	nets := []netRequire{
		netRequire{
			Type: _ContainersNetworking,
			num:  1,
		},
	}
	return require{
		ports:       ports,
		networkings: nets,
	}
}

func (c switchManagerConfig) HealthCheck() (healthCheck, error) {
	if c.config == nil {
		return healthCheck{}, fmt.Errorf("params not ready")
	}

	port, err := c.config.Int("Port")
	if err != nil {
		return healthCheck{}, err
	}
	return healthCheck{
		Port:     port,
		Script:   "/opt/DBaaS/script/check_switchmanager.sh ",
		Shell:    "",
		Interval: "10s",
		TTL:      "15s",
		Tags:     nil,
	}, nil
}
func (c switchManagerConfig) defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error) {
	sys, err := database.GetSystemConfig()
	if err != nil {
		return nil, err
	}

	m := make(map[string]interface{}, 10)
	m["domain"] = svc.ID
	m["name"] = u.Name
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

	return m, nil
}
