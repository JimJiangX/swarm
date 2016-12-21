package swarm

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/astaxie/beego/config"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
	"github.com/docker/swarm/version"
	"github.com/pkg/errors"
)

// ContainerCmd commands of actions
type containerCmd interface {
	StartContainerCmd() []string
	InitServiceCmd(args ...string) []string
	StartServiceCmd() []string
	StopServiceCmd() []string
	RestoreCmd(file, backupDir string) []string
	BackupCmd(args ...string) []string
	CleanBackupFileCmd(args ...string) []string
}

type port struct {
	port  int
	proto string
	name  string
}

type netRequire struct {
	Type string
	num  int
}

type require struct {
	ports       []port
	networkings []netRequire
}

type configParser interface {
	Validate(data map[string]interface{}) error
	ParseData(data []byte) error
	defaultUserConfig(args ...interface{}) (map[string]interface{}, error)
	Marshal() ([]byte, error)
	Requirement() require
	HealthCheck(args ...string) (healthCheck, error)
	Set(key string, val interface{}) error
	String(key string) string
}

func (u unit) Path() string {
	if u.parent == nil {
		return "/"
	}

	return u.parent.Mount
}

func (u unit) CanModify(data map[string]interface{}) ([]string, bool) {
	can := true
	keys := make([]string, 0, len(data))

	for key := range data {
		// case sensitive
		if len(u.parent.KeySets) == 0 ||
			!u.parent.KeySets[strings.ToLower(key)].CanSet {

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

func (u unit) verify(data map[string]interface{}) error {
	if len(data) > 0 {
		err := u.Validate(data)
		if err != nil {
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
		return restart, errors.Errorf("key %s cannot set new value", key)
	}

	if u.parent.KeySets[key].MustRestart {
		restart = true
	}

	return restart, u.setServiceConfig(key, val)
}

func (u *unit) setServiceConfig(key string, val interface{}) error {
	if u.configParser == nil {
		return errors.Errorf("Unit %s configParser is nil", u.Name)
	}

	return u.configParser.Set(key, val)
}

func (u *unit) saveConfigToDisk(content []byte) error {
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
	config.UnitID = u.ID

	return database.SaveUnitConfig(&u.Unit, config)
}

// Factory returns configParser and containerCmd,
// returns a error if name&version unsupported
func Factory(name, version string) (configParser, containerCmd, error) {
	return initialize(name, version)
}

func initialize(name, version string) (parser configParser, cmder containerCmd, err error) {
	switch {

	case _ImageProxy == name && version == "1.0.2":
		parser = &proxyConfigV102{}
		cmder = &proxyCmd{}

	case _ImageProxy == name && version == "1.1.0":
		parser = &proxyConfigV110{}
		cmder = &proxyCmd{}

	case _ImageSwitchManager == name && version == "1.1.19":
		parser = &switchManagerConfigV1119{}
		cmder = &switchManagerCmd{}

	case _ImageSwitchManager == name && version == "1.1.23":
		parser = &switchManagerConfigV1123{}
		cmder = &switchManagerCmd{}

	case _ImageProxy == name:
		parser = &proxyConfig{}
		cmder = &proxyCmd{}

	case _ImageSwitchManager == name:
		parser = &switchManagerConfig{}
		cmder = &switchManagerCmd{}

	case _ImageUpsql == name && strings.HasPrefix(version, "5.6"):
		parser = &mysqlConfig{}
		cmder = &mysqlCmd{}

	case _ImageUpsql == name && strings.HasPrefix(version, "1."):
		parser = &mysqlConfig{}
		cmder = &mysqlCmd{}

	case _ImageRedis == name:
		parser = &redisConfig{}
		cmder = &redisCmd{}

	default:

		return nil, nil, errors.Errorf("unsupported Image:'%s:%s'", name, version)
	}

	return parser, cmder, nil
}

type mysqlCmd struct{}

func (mysqlCmd) StartContainerCmd() []string {
	return []string{"/bin/bash"}
}
func (mysqlCmd) InitServiceCmd(args ...string) []string {
	cmd := make([]string, len(args)+1)
	cmd[0] = "/root/upsql-init.sh"
	copy(cmd[1:], args)

	return cmd
}
func (mysqlCmd) StartServiceCmd() []string {
	return []string{"/root/upsql.service", "start"}
}
func (mysqlCmd) StopServiceCmd() []string {
	return []string{"/root/upsql.service", "stop"}
}
func (mysqlCmd) RestoreCmd(file, backupDir string) []string {
	return []string{"/root/upsql-restore.sh", file, backupDir}
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

func (c mysqlConfig) defaultUserConfig(args ...interface{}) (_ map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
	}()

	errMsg := fmt.Sprintf("unexpected args,len=%d", len(args))

	if len(args) < 2 {
		return nil, errors.New(errMsg)
	}
	svc, ok := args[0].(*Service)
	if !ok || svc == nil {
		return nil, errors.New(errMsg)
	}

	u, ok := args[1].(*unit)
	if !ok || u == nil {
		return nil, errors.New(errMsg)
	}

	m := make(map[string]interface{}, 10)

	if len(u.networkings) == 1 {
		m["mysqld::bind_address"] = u.networkings[0].IP.String()
	} else {
		return nil, errors.New("unexpected IPAddress")
	}

	found := false
	for i := range u.ports {
		if u.ports[i].Name == "mysqld::port" {
			m["mysqld::port"] = u.ports[i].Port
			m["mysqld::server_id"] = u.ports[i].Port
			found = true
		}
	}
	if !found {
		return nil, errors.New("unexpected port allocation")
	}

	m["mysqld::log_bin"] = fmt.Sprintf("/DBAASLOG/BIN/%s-binlog", u.Name)
	m["mysqld::innodb_buffer_pool_size"] = int(float64(u.config.HostConfig.Memory) * 0.75)
	m["mysqld::relay_log"] = fmt.Sprintf("/DBAASLOG/REL/%s-relay", u.Name)

	m["client::user"] = ""
	m["client::password"] = ""

	users, err := svc.getUsers()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Service": svc.Name,
			"Unit":    u.Name,
		}).Errorf("get Service users,%+v", err)

	} else {

		for i := range users {
			if users[i].Role == _User_DBA_Role {
				m["client::user"] = users[i].Username
				m["client::password"] = users[i].Password

				break
			}
		}
	}

	return m, nil
}

func (c *mysqlConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("mysqlConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c mysqlConfig) String(key string) string {
	if c.config == nil {
		return ""
	}

	return c.config.String(strings.ToLower(key))
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
	Addr              string
	Port              int
	Script            string
	Shell             string
	Interval          string
	Timeout           string
	TTL               string
	Tags              []string
	TCP               string
	HTTP              string
	DockerContainerID string
	Status            string
}

func (c mysqlConfig) HealthCheck(args ...string) (healthCheck, error) {
	if c.config == nil || len(args) < 3 {
		return healthCheck{}, errors.New("params not ready")
	}

	addr := c.config.String("mysqld::bind_address")
	port, err := c.config.Int("mysqld::port")
	if err != nil {
		return healthCheck{}, errors.Wrap(err, "get 'mysqld::port'")
	}
	return healthCheck{
		Addr:     addr,
		Port:     port,
		Script:   "/opt/DBaaS/script/check_db.sh " + args[0] + " " + args[1] + " " + args[2],
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
func (proxyCmd) InitServiceCmd(args ...string) []string {
	return []string{"/root/upproxy.service", "start"}
}
func (proxyCmd) StartServiceCmd() []string {
	return []string{"/root/upproxy.service", "start"}
}
func (proxyCmd) StopServiceCmd() []string {
	return []string{"/root/upproxy.service", "stop"}
}
func (proxyCmd) RestoreCmd(file, backupDir string) []string { return nil }
func (proxyCmd) BackupCmd(args ...string) []string          { return nil }
func (proxyCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type proxyConfig struct {
	config config.Configer
}

func (c *proxyConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("mysqlConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c proxyConfig) String(key string) string {
	if c.config == nil {
		return ""
	}

	return c.config.String(strings.ToLower(key))
}

func (proxyConfig) Validate(data map[string]interface{}) error { return nil }
func (c *proxyConfig) ParseData(data []byte) error {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return errors.Wrap(err, "parse ini")
	}

	c.config = configer

	return nil
}

func (c *proxyConfig) Marshal() ([]byte, error) {
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, errors.Wrap(err, "create tempFile")
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

func (c proxyConfig) HealthCheck(args ...string) (healthCheck, error) {
	if c.config == nil || len(args) == 0 {
		return healthCheck{}, errors.New("params not ready")
	}

	addr := c.config.String("adm-cli::adm-cli-address")
	port, err := c.config.Int("adm-cli::proxy_admin_port")
	if err != nil {
		return healthCheck{}, errors.Wrap(err, "get 'adm-cli::proxy_admin_port'")
	}

	return healthCheck{
		Addr:     addr,
		Port:     port,
		Script:   "/opt/DBaaS/script/check_proxy.sh " + args[0],
		Shell:    "",
		Interval: "10s",
		TTL:      "",
		Tags:     nil,
	}, nil
}

func (c proxyConfig) defaultUserConfig(args ...interface{}) (_ map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
	}()

	errMsg := fmt.Sprintf("unexpected args,len=%d", len(args))

	if len(args) < 2 {
		return nil, errors.New(errMsg)
	}
	svc, ok := args[0].(*Service)
	if !ok || svc == nil {
		return nil, errors.New(errMsg)
	}

	u, ok := args[1].(*unit)
	if !ok || u == nil {
		return nil, errors.New(errMsg)
	}

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

	ncpu, err := utils.GetCPUNum(u.config.HostConfig.CpusetCpus)
	if err == nil {
		m["upsql-proxy::event-threads-count"] = ncpu
	} else {
		logrus.WithError(err).Warnf("%s upsql-proxy::event-threads-count", u.Name)
		m["upsql-proxy::event-threads-count"] = 1
	}

	swm, err := svc.getSwithManagerUnit()
	if err == nil && swm != nil {
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

type proxyConfigV102 struct {
	proxyConfig
}

type proxyConfigV110 struct {
	proxyConfig
}

func (c proxyConfigV110) defaultUserConfig(args ...interface{}) (_ map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
	}()

	errMsg := fmt.Sprintf("unexpected args,len=%d", len(args))

	if len(args) < 2 {
		return nil, errors.New(errMsg)
	}
	svc, ok := args[0].(*Service)
	if !ok || svc == nil {
		return nil, errors.New(errMsg)
	}

	u, ok := args[1].(*unit)
	if !ok || u == nil {
		return nil, errors.New(errMsg)
	}

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
		m["supervise::supervise-address"] = fmt.Sprintf("%s:%d", dataAddr, adminPort)
		m["adm-cli::adm-cli-address"] = fmt.Sprintf("%s:%d", adminAddr, adminPort)
	}

	ncpu, err := utils.GetCPUNum(u.config.HostConfig.CpusetCpus)
	if err == nil {
		m["upsql-proxy::event-threads-count"] = ncpu
	} else {
		logrus.WithError(err).Warnf("%s upsql-proxy::event-threads-count", u.Name)
		m["upsql-proxy::event-threads-count"] = 1
	}

	swm, err := svc.getSwithManagerUnit()
	if err == nil && swm != nil {
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
func (switchManagerCmd) InitServiceCmd(args ...string) []string {
	return []string{"/root/swm.service", "start"}
}
func (switchManagerCmd) StartServiceCmd() []string {
	return []string{"/root/swm.service", "start"}
}
func (switchManagerCmd) StopServiceCmd() []string {
	return []string{"/root/swm.service", "stop"}
}
func (switchManagerCmd) RestoreCmd(file, backupDir string) []string { return nil }
func (switchManagerCmd) BackupCmd(args ...string) []string          { return nil }
func (switchManagerCmd) CleanBackupFileCmd(args ...string) []string { return nil }

type switchManagerConfig struct {
	config config.Configer
}

func (c *switchManagerConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		return errors.New("switchManagerConfig Configer is nil")
	}

	return c.config.Set(strings.ToLower(key), fmt.Sprintf("%v", val))
}

func (c switchManagerConfig) String(key string) string {
	if c.config == nil {
		return ""
	}

	return c.config.String(strings.ToLower(key))
}

func (switchManagerConfig) Validate(data map[string]interface{}) error { return nil }
func (c *switchManagerConfig) ParseData(data []byte) error {
	configer, err := config.NewConfigData("ini", data)
	if err != nil {
		return errors.Wrap(err, "parse ini")
	}

	c.config = configer

	return nil
}

func (c *switchManagerConfig) Marshal() ([]byte, error) {
	tmpfile, err := ioutil.TempFile("", "serviceConfig")
	if err != nil {
		return nil, errors.Wrap(err, "create tempFile")
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

func (c switchManagerConfig) HealthCheck(args ...string) (healthCheck, error) {
	if c.config == nil || len(args) == 0 {
		return healthCheck{}, errors.New("params not ready")
	}

	port, err := c.config.Int("Port")
	if err != nil {
		return healthCheck{}, errors.Wrap(err, "get 'Port'")
	}

	return healthCheck{
		Addr:     "",
		Port:     port,
		Script:   "/opt/DBaaS/script/check_switchmanager.sh " + args[0],
		Shell:    "",
		Interval: "10s",
		TTL:      "",
		Tags:     nil,
	}, nil
}

func (c switchManagerConfig) defaultUserConfig(args ...interface{}) (_ map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
	}()

	errMsg := fmt.Sprintf("unexpected args,len=%d", len(args))

	if len(args) < 2 {
		return nil, errors.New(errMsg)
	}
	svc, ok := args[0].(*Service)
	if !ok || svc == nil {
		return nil, errors.New(errMsg)
	}

	u, ok := args[1].(*unit)
	if !ok || u == nil {
		return nil, errors.New(errMsg)
	}

	user, err := svc.getUserByRole(_User_Check_Role)
	if err != nil {
		return nil, err
	}

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
	m["ConsulPort"] = sys.ConsulPort

	// swarm
	m["SwarmUserAgent"] = version.VERSION
	m["SwarmHostKey"] = leaderElectionPath

	// _User_Check Role
	m["swarmhealthcheckuser"] = user.Username
	m["swarmhealthcheckpassword"] = user.Password

	return m, nil
}

type switchManagerConfigV1119 struct {
	switchManagerConfig
}

type switchManagerConfigV1123 struct {
	switchManagerConfig
}

func (c switchManagerConfigV1123) defaultUserConfig(args ...interface{}) (_ map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
	}()

	errMsg := fmt.Sprintf("unexpected args,len=%d", len(args))

	if len(args) < 2 {
		return nil, errors.New(errMsg)
	}
	svc, ok := args[0].(*Service)
	if !ok || svc == nil {
		return nil, errors.New(errMsg)
	}

	u, ok := args[1].(*unit)
	if !ok || u == nil {
		return nil, errors.New(errMsg)
	}

	user, err := svc.getUserByRole(_User_Check_Role)
	if err != nil {
		return nil, err
	}

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
	m["ConsulPort"] = sys.ConsulPort

	// swarm
	m["SwarmUserAgent"] = version.VERSION
	m["SwarmHostKey"] = leaderElectionPath

	// _User_Check Role
	m["swarmhealthcheckuser"] = user.Username
	m["swarmhealthcheckpassword"] = user.Password

	return m, nil
}

type redisConfig struct {
	config map[string]string
}

func (c redisConfig) Validate(data map[string]interface{}) error {
	return nil
}

func (c *redisConfig) ParseData(data []byte) error {
	if c.config == nil {
		c.config = make(map[string]string, 20)
	}

	lines := bytes.Split(data, []byte{'\n'})
	for _, l := range lines {
		if bytes.HasPrefix(l, []byte{'#'}) {
			continue
		}

		line := bytes.TrimSpace(l)
		if len(line) == 0 {
			continue
		}

		parts := bytes.SplitN(line, []byte{' '}, 2)
		if len(parts) == 2 {
			c.config[string(parts[0])] = string(parts[1])
		}
	}

	return nil
}

func (redisConfig) defaultUserConfig(args ...interface{}) (_ map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}
	}()

	errMsg := fmt.Sprintf("unexpected args,len=%d", len(args))

	if len(args) < 2 {
		return nil, errors.New(errMsg)
	}

	u, ok := args[1].(*unit)
	if !ok || u == nil {
		return nil, errors.New(errMsg)
	}

	var (
		m    = make(map[string]interface{}, 10)
		port = 0
	)
	for i := range u.ports {
		if u.ports[i].Name == "port" {
			port = u.ports[i].Port
			break
		}
	}
	m["port"] = port

	if len(u.networkings) == 1 {
		m["bind"] = u.networkings[0].IP.String()
	}

	m["maxmemory"] = u.config.HostConfig.Resources.Memory

	return m, nil
}

func (c redisConfig) Marshal() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	for key, val := range c.config {
		_, err := buffer.WriteString(key)
		if err != nil {
			return buffer.Bytes(), err
		}

		err = buffer.WriteByte(' ')
		if err != nil {
			return buffer.Bytes(), err
		}

		_, err = buffer.WriteString(val)
		if err != nil {
			return buffer.Bytes(), err
		}

		err = buffer.WriteByte('\n')
		if err != nil {
			return buffer.Bytes(), err
		}
	}

	return buffer.Bytes(), nil
}

func (redisConfig) Requirement() require {
	ports := []port{
		port{
			proto: "tcp",
			name:  "port",
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

func (c redisConfig) HealthCheck(args ...string) (healthCheck, error) {
	if c.config == nil {
		return healthCheck{}, errors.New("params not ready")
	}

	addr := c.config["bind"]
	port, err := strconv.Atoi(c.config["port"])
	if err != nil {
		return healthCheck{}, errors.Wrap(err, "get 'Port'")
	}

	return healthCheck{
		Addr: addr,
		Port: port,
		// Script:   "/opt/DBaaS/script/check_switchmanager.sh " + args[0],
		Shell:    "",
		Interval: "10s",
		TTL:      "",
		Tags:     nil,
		TCP:      addr + ":" + c.config["port"],
	}, nil
}

func (c *redisConfig) Set(key string, val interface{}) error {
	if c.config == nil {
		c.config = make(map[string]string, 20)
	}

	c.config[strings.ToLower(key)] = fmt.Sprintf("%v", val)

	return nil
}

func (c redisConfig) String(key string) string {
	if c.config == nil {
		return ""
	}

	return c.config[strings.ToLower(key)]
}

type redisCmd struct{}

func (redisCmd) StartContainerCmd() []string {
	return []string{"bin/bash"}
}

func (redisCmd) InitServiceCmd(args ...string) []string {
	return []string{"/root/redis.service", "start"}
}

func (redisCmd) StartServiceCmd() []string {
	return []string{"/root/redis.service", "start"}
}

func (redisCmd) StopServiceCmd() []string {
	return []string{"/root/redis.service", "stop"}
}

func (redisCmd) RestoreCmd(file, backupDir string) []string {
	return nil
}

func (redisCmd) BackupCmd(args ...string) []string {
	return nil
}

func (redisCmd) CleanBackupFileCmd(args ...string) []string {
	return nil
}
