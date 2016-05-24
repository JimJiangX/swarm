package swarm

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/astaxie/beego/config"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	consulapi "github.com/hashicorp/consul/api"
	"golang.org/x/net/context"
)

type ContainerCmd interface {
	StartContainerCmd() []string
	InitServiceCmd() []string
	StartServiceCmd() []string
	StopServiceCmd() []string
	RecoverCmd(file string) []string
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
	ParseData(data []byte) (config.Configer, error)
	defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error)
	Marshal() ([]byte, error)
	Requirement() require
	HealthCheck() (healthCheck, error)
	Set(key string, val interface{}) error
}

type unit struct {
	database.Unit
	engine      *cluster.Engine
	config      *cluster.ContainerConfig
	container   *cluster.Container
	parent      *database.UnitConfig
	ports       []database.Port
	networkings []IPInfo

	configParser
	ContainerCmd
}

func (u *unit) factory() error {
	parser, cmder, err := initialize(u.Type)
	if err != nil || parser == nil || cmder == nil {
		return fmt.Errorf("Unexpected Error,%s:%v", u.Type, err)
	}

	u.configParser = parser
	u.ContainerCmd = cmder

	return nil
}

func (gd *Gardener) rebuildUnit(table database.Unit) (unit, error) {
	u := unit{
		Unit: table,
	}
	c := gd.Container(table.ContainerID)
	if c == nil {
		c = gd.Container(table.Name)
	}
	if c != nil {
		u.container = c
		u.engine = c.Engine
		u.config = c.Config
	}

	if u.engine == nil && u.NodeID != "" {
		node, err := gd.GetNode(u.NodeID)
		if err != nil {
			logrus.Errorf("Not Found Node %s,Error:%s", u.NodeID, err.Error())
		} else if node != nil && node.engine != nil {
			u.engine = node.engine
		}
	}

	if u.ConfigID != "" {
		config, err := database.GetUnitConfigByID(u.ConfigID)
		if err == nil {
			u.parent = config
		} else {
			logrus.Errorf("Cannot Query unit Parent Config By ConfigID %s,Error:%s", u.ConfigID, err.Error())
		}

	}

	ports, err := database.GetPortsByUnit(u.ID)
	if err == nil {
		u.ports = ports
	} else {
		logrus.Errorf("Cannot Query unit ports By UnitID %s,Error:%s", u.ID, err.Error())
	}

	u.networkings, err = getIPInfoByUnitID(u.ID)
	if err != nil {
		logrus.Errorf("Cannot Query unit networkings By UnitID %s,Error:%s", u.ID, err.Error())
	}

	if err := u.factory(); err != nil {
		return u, err
	}

	return u, nil
}

func (u *unit) getNetworkings() ([]IPInfo, error) {
	if len(u.networkings) > 0 {
		return u.networkings, nil
	}
	networkings, err := getIPInfoByUnitID(u.ID)
	if err != nil {
		return nil, fmt.Errorf("Cannot Query unit networkings By UnitID %s,Error:%s", u.ID, err.Error())
	}

	u.networkings = networkings

	return u.networkings, nil
}

func (u *unit) getNetworkingAddr(networking, portName string) (addr string, port int, err error) {
	for i := range u.networkings {
		if u.networkings[i].Type == networking {
			addr = u.networkings[i].IP.String()

			break
		}
	}

	for i := range u.ports {
		if u.ports[i].Name == "Port" {
			port = u.ports[i].Port

			return addr, port, nil
		}
	}

	return "", 0, fmt.Errorf("Not Found Required networking:%s Port:%s", networking, portName)
}

func (u *unit) prepareCreateContainer() error {
	for i := range u.networkings {
		n := u.networkings[i]
		err := u.createNetworking(n.IP.String(), n.Device, n.Prefix)
		if err != nil {
			return err
		}
	}

	// prepare for volumes
	binds := u.config.HostConfig.Binds

	for _, name := range binds {
		parts := strings.SplitN(name, ":", 2)
		lv, err := database.GetLocalVoume(parts[0])
		if err != nil {
			return err
		}

		// if volume create on san storage,should created VG before create Volume
		if strings.Contains(lv.VGName, "_SAN_VG") {
			err := u.createSanStoreageVG(parts[0])
			if err != nil {
				return err
			}
		}

		req := &types.VolumeCreateRequest{
			Name:       lv.Name,
			Driver:     lv.Driver,
			DriverOpts: map[string]string{"size": strconv.Itoa(lv.Size), "fstype": lv.Filesystem, "vgname": lv.VGName},
			Labels:     nil,
		}
		_, err = u.engine.CreateVolume(req)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u *unit) pullImage(authConfig *types.AuthConfig) error {
	if u.config.Image == "" || u.engine == nil {
		return fmt.Errorf("params error,image:%s,Engine:%+v", u.config.Image, u.engine)
	}

	err := u.engine.Pull(u.config.Image, authConfig)
	if err != nil {
		// try again
		err := u.engine.Pull(u.config.Image, authConfig)
		if err != nil {
			return err
		}
	}

	if image := u.engine.Image(u.config.Image); image != nil {
		return nil
	}

	return err
}

func (u *unit) createLocalDiskVolume(name string) (*cluster.Volume, error) {
	lv, err := database.GetLocalVoume(name)
	if err != nil {
		return nil, err
	}

	req := &types.VolumeCreateRequest{
		Name:       lv.Name,
		Driver:     lv.Driver,
		DriverOpts: map[string]string{"size": strconv.Itoa(lv.Size), "fstype": lv.Filesystem, "vgname": lv.VGName},
		Labels:     nil,
	}
	v, err := u.engine.CreateVolume(req)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (u *unit) createSanStoreageVG(name string) error {
	list, err := database.ListLUNByName(name)
	if err != nil || len(list) == 0 {
		return fmt.Errorf("LUN:%d,Error:%v", len(list), err)
	}

	storage, err := store.GetStoreByID(list[0].StorageSystemID)
	if err != nil {
		return err
	}

	l, size := make([]int, len(list)), 0
	for i := range list {
		l[i] = list[i].StorageLunID
		size += list[i].SizeByte
	}

	config := sdk.VgConfig{
		HostLunId: l,
		VgName:    list[0].VGName,
		Type:      storage.Vendor(),
	}

	addr := u.getPluginAddr(pluginPort)

	return sdk.SanVgCreate(addr, config)
}

func (u *unit) createContainer(authConfig *types.AuthConfig) (*cluster.Container, error) {
	container, err := u.engine.Create(u.config, u.Unit.ID, true, authConfig)
	if err == nil && container != nil {
		u.container = container
		u.Unit.ContainerID = container.ID
	}

	return container, err
}

func (u *unit) updateContainer(updateConfig container.UpdateConfig) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerUpdate(context.TODO(), u.container.ID, updateConfig)
}

func (u *unit) removeContainer(force, rmVolumes bool) error {
	err := u.engine.RemoveContainer(u.container, force, rmVolumes)

	return err
}

func (u *unit) startContainer() error {
	return u.engine.StartContainer(u.Unit.Name, nil)
}

func (u *unit) stopContainer(timeout int) error {
	u.stopService()

	client := u.engine.EngineAPIClient()

	return client.ContainerStop(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) restartContainer(timeout int) error {
	err := u.stopService()
	if err != nil {
		return err
	}

	client := u.engine.EngineAPIClient()

	return client.ContainerRestart(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) renameContainer(name string) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerRename(context.TODO(), u.container.ID, u.Unit.ID)
}

func (u *unit) createNetworking(ip, device string, prefix int) error {
	config := sdk.IPDevConfig{
		Device: device,
		IPCIDR: fmt.Sprintf("%s/%d", ip, prefix),
	}

	addr := u.getPluginAddr(pluginPort)

	return sdk.CreateIP(addr, config)
}

func (u *unit) removeNetworking(ip, device string, prefix int) error {
	config := sdk.IPDevConfig{
		Device: device,
		IPCIDR: fmt.Sprintf("%s/%d", ip, prefix),
	}

	addr := u.getPluginAddr(pluginPort)

	return sdk.RemoveIP(addr, config)
}

func (u *unit) createVolume() (*cluster.Volume, error) {
	return nil, nil
}

func (u *unit) updateVolume() error {
	return nil
}

func (u *unit) removeVolume() error {
	return nil
}

func (u *unit) createVG() error {
	return nil
}

func (u *unit) activateVG() error {
	return nil
}

func (u *unit) deactivateVG() error {
	return nil
}

func (u *unit) extendVG() error {
	return nil
}

func (u *unit) RegisterHealthCheck(config database.ConsulConfig, context *Service) error {
	address := ""
	if u.engine != nil {
		address = fmt.Sprintf("%s:%d", u.engine.IP, config.ConsulPort)
	} else {
		node, err := database.GetNode(u.NodeID)
		if err != nil {
			logrus.Errorf("Not Found Node %s,Error:%s", u.NodeID, err.Error())
			return err
		}
		address = fmt.Sprintf("%s:%d", node.Addr, config.ConsulPort)
	}
	c := consulapi.Config{
		Address:    address,
		Datacenter: config.ConsulDatacenter,
		WaitTime:   time.Duration(config.ConsulWaitTime) * time.Second,
		Token:      config.ConsulToken,
	}
	client, err := consulapi.NewClient(&c)
	if err != nil {
		logrus.Errorf("%s Register HealthCheck Error,%s %v", u.Name, err.Error(), c)
		return err
	}

	check, err := u.HealthCheck()
	if err != nil {
		return err
	}

	if u.Type == _UpsqlType {
		swm, err := context.getUnitByType(_UnitRole_SwitchManager)
		if err == nil && swm != nil {
			check.Tags = []string{fmt.Sprintf("swm_key=%s/%s/topology", context.ID, swm.ID)}
		}
	}

	containerID := u.ContainerID
	if u.container != nil && containerID != u.container.ID {
		containerID = u.container.ID
		u.ContainerID = u.container.ID
	}

	addr := ""
	ips, err := u.getNetworkings()
	if err != nil {
		return err
	}
	for i := range ips {
		if ips[i].Type == _ContainersNetworking {
			addr = ips[i].IP.String()
		}
	}

	service := consulapi.AgentServiceRegistration{
		ID:      u.ID,
		Name:    u.Name,
		Tags:    check.Tags,
		Port:    check.Port,
		Address: addr,
		Check: &consulapi.AgentServiceCheck{
			Script: check.Script + u.Name,
			// DockerContainerID: containerID,
			Shell:    check.Shell,
			Interval: check.Interval,
			// TTL:      check.TTL,
		},
	}

	logrus.Debugf("AgentServiceRegistration:%v %v", service, service.Check)

	return client.Agent().ServiceRegister(&service)
}

func (u *unit) DeregisterHealthCheck(client *consulapi.Client) error {
	if client == nil {
		return fmt.Errorf("consul client is nil")
	}

	return client.Agent().ServiceDeregister(u.ID)
}

func (u *unit) Migrate(e *cluster.Engine, config *cluster.ContainerConfig) (*cluster.Container, error) {
	return nil, nil
}

func (u *unit) CopyConfig(data map[string]interface{}) error {
	_, err := u.ParseData([]byte(u.parent.Content))
	if err != nil {
		return err
	}

	err = u.Verify(data)
	if err != nil {
		return err
	}

	for key, val := range data {
		if err := u.Set(key, val); err != nil {
			return err
		}
	}

	content, err := u.Marshal()
	if err != nil {
		return err
	}

	if err := u.SaveConfigToDisk(content); err != nil {
		return err
	}

	volumes, err := database.SelectVolumesByUnitID(u.ID)
	if err != nil {
		return err
	}

	cnf := 0
	for i := range volumes {
		if strings.Contains(volumes[i].Name, "_CNF_LV") {
			cnf = i
			break
		}
		if strings.Contains(volumes[i].Name, "_DAT_LV") {
			cnf = i
		}
	}

	path := u.Path()
	if strings.HasPrefix(path, "/DBAAS") {
		parts := strings.SplitN(path, "/", 3)
		if len(parts) == 3 {
			path = parts[2]
		}
	}

	config := sdk.VolumeFileConfig{
		VgName:    volumes[cnf].VGName,
		LvsName:   volumes[cnf].Name,
		MountName: volumes[cnf].Name,
		Data:      string(content),
		FDes:      path,
		Mode:      "0666",
	}

	logrus.Debugf("default:%s\ndefaultUser:%v\nconfig:%s", u.parent.Content, data, config.Data)

	logrus.Debugf("VolumeFileConfig:%+v", config)
	err = sdk.FileCopyToVolome(u.getPluginAddr(pluginPort), config)

	return err
}

func (u *unit) initService() error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.InitServiceCmd()
	if len(cmd) == 0 {
		logrus.Warnf("%s InitServiceCmd is nil", u.Name)
		return nil
	}

	inspect, err := containerExec(u.engine, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = fmt.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}
	if err != nil {
		return err
	}

	return nil
}

func (u *unit) startService() error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.StartServiceCmd()
	if len(cmd) == 0 {
		logrus.Warnf("%s StartServiceCmd is nil", u.Name)
		return nil
	}

	inspect, err := containerExec(u.engine, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = fmt.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}
	if err != nil {
		return err
	}

	return nil
}

func (u *unit) stopService() error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.StopServiceCmd()
	if len(cmd) == 0 {
		logrus.Warnf("%s StopServiceCmd is nil", u.Name)
		return nil
	}

	inspect, err := containerExec(u.engine, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = fmt.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}
	if err != nil {
		return err
	}

	return nil
}

func (u *unit) backup(args ...string) error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.BackupCmd(args...)
	if len(cmd) == 0 {
		logrus.Warnf("%s BackupCmd is nil", u.Name)
		return nil
	}
	logrus.WithFields(logrus.Fields{
		"Name": u.Name,
		"Cmd":  cmd,
	}).Debugln("start Backup job")

	inspect, err := containerExec(u.engine, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = fmt.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}
	if err != nil {
		return err
	}

	return nil
}

var pluginPort = 3333

func (u unit) getPluginAddr(port int) string {

	return fmt.Sprintf("%s:%d", u.engine.IP, port)
}

func (u *unit) saveToDisk() error {

	return database.UpdateUnitInfo(u.Unit)
}

func (u *unit) deregisterInHorus() error { return nil }

func (u *unit) registerHorus(user, password string, agentPort int) (registerService, error) {
	node, err := database.GetNode(u.NodeID)
	if err != nil {
		return registerService{}, err
	}

	typ := u.Type
	switch u.Type {
	case _SwitchManagerType, "switch manager", "switchmanager":
		typ = "swm"
	case _ProxyType, "upproxy":
		typ = "upproxy"
	case _MysqlType, _UpsqlType:
		typ = "upsql"
	default:
		return registerService{}, fmt.Errorf("Unsupported 'Type':'%s'", u.Type)
	}

	return registerService{
		Endpoint:      u.ID,
		CollectorName: u.Name,
		User:          user,
		Password:      password,
		Type:          typ,
		CollectorIP:   u.engine.IP,
		CollectorPort: agentPort,
		MetricTags:    node.ID,
		Network:       nil,
		Status:        "on",
		Table:         "host",
		CheckType:     "health",
	}, nil
}

func (gd *Gardener) unitContainer(u *unit) *cluster.Container {
	ID := u.ContainerID
	if u.container != nil && u.ContainerID != u.container.ID {
		ID = u.container.ID
		u.ContainerID = u.container.ID
	}
	c := gd.Container(ID)
	if c != nil {
		u.container = c
	}

	return u.container
}
