package swarm

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/astaxie/beego/config"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
	consulapi "github.com/hashicorp/consul/api"
	"golang.org/x/net/context"
)

var (
	errEngineIsNil    = errors.New("Engine is nil")
	errEngineAPIisNil = errors.New("Engine API client is nil")
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

func (u *unit) getEngine() (*cluster.Engine, error) {
	if u.engine != nil {
		return u.engine, nil
	}

	if u.container != nil && u.container.Engine != nil {
		u.engine = u.container.Engine
		return u.engine, nil
	}

	return nil, errEngineIsNil
}

func (u *unit) getEngineAPIClient() (client.APIClient, error) {
	eng, err := u.getEngine()
	if err != nil {
		return nil, err
	}

	if eng == nil {
		return nil, errEngineIsNil
	}

	client := eng.EngineAPIClient()
	if client == nil {
		return nil, errEngineAPIisNil
	}

	return client, nil
}

func (gd *Gardener) GetUnit(table database.Unit) (*unit, error) {
	var (
		svc *Service
		u   *unit
	)
	gd.RLock()
	for i := range gd.services {
		if gd.services[i].ID == table.ServiceID {
			svc = gd.services[i]
			break
		}
	}
	gd.RUnlock()

	if svc != nil {
		svc.RLock()
		u, _ = svc.getUnit(table.ID)
		svc.RUnlock()
	}

	if u == nil || u.engine == nil {
		value, err := gd.rebuildUnit(table)
		if err != nil {
			return nil, err
		}

		u = &value
	}

	return u, nil
}

func (gd *Gardener) rebuildUnit(table database.Unit) (unit, error) {
	var c *cluster.Container
	u := unit{
		Unit: table,
	}

	if table.ContainerID != "" {
		c = gd.Container(table.ContainerID)
		if c == nil {
			c = gd.Container(table.Name)
		}
	}
	if c != nil {
		u.container = c
		u.engine = c.Engine
		u.config = c.Config
	}

	if u.engine == nil && u.EngineID != "" {
		node, err := gd.GetNode(u.EngineID)
		if err != nil {
			logrus.Errorf("Not Found Node %s,Error:%s", u.EngineID, err.Error())
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

	u.networkings, err = u.getNetworkings()
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
	networkings, err := getIPInfoByUnitID(u.ID, u.engine)
	if err != nil {
		return nil, fmt.Errorf("Cannot Query unit networkings By UnitID %s,Error:%s", u.ID, err)
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
		if u.ports[i].Name == portName {

			return addr, u.ports[i].Port, nil
		}
	}

	return "", 0, fmt.Errorf("Not Found Required networking:%s Port:%s", networking, portName)
}

func (u *unit) prepareCreateContainer() error {
	err := u.createNetworking()
	if err != nil {
		return err
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

	if image := u.engine.Image(u.config.Image); image == nil {
		return fmt.Errorf("Not Found Image %s On Engine %s:%s", u.config.Image, u.engine.ID, u.engine.Addr)
	}

	return nil
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
	client, err := u.getEngineAPIClient()
	if err != nil {
		return err
	}

	return client.ContainerUpdate(context.TODO(), u.container.ID, updateConfig)
}

func (u *unit) removeContainer(force, rmVolumes bool) error {
	engine := u.engine
	if engine == nil {
		if u.container == nil || u.container.Engine == nil {
			return fmt.Errorf("Unit %s Engine is null", u.Name)
		} else {
			engine = u.container.Engine
		}
	}

	c := u.container
	if c == nil {
		c = &cluster.Container{
			Container: types.Container{
				ID: u.ContainerID,
			}}
		if u.ContainerID == "" {
			c.ID = u.Name
		}
	}

	err := engine.RemoveContainer(c, force, rmVolumes)
	if err != nil {
		return err
	}

	err = u.removeNetworking()

	return err
}

func (u *unit) startContainer() error {
	if u.engine == nil {
		return errEngineIsNil
	}
	return u.engine.StartContainer(u.Unit.Name, nil)
}

func (u *unit) stopContainer(timeout int) error {
	u.stopService()

	client, err := u.getEngineAPIClient()
	if err != nil {
		return err
	}

	return client.ContainerStop(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) restartContainer(timeout int) error {
	err := u.stopService()
	if err != nil {
		return err
	}

	if u.engine == nil {
		return errEngineIsNil
	}

	client := u.engine.EngineAPIClient()
	if client == nil {
		return errEngineAPIisNil
	}

	return client.ContainerRestart(context.TODO(), u.Unit.ContainerID, timeout)
}

func (u *unit) renameContainer(name string) error {
	client, err := u.getEngineAPIClient()
	if err != nil {
		return err
	}

	return client.ContainerRename(context.TODO(), u.container.ID, u.Unit.ID)
}

func (u *unit) createNetworking() error {
	addr := u.getPluginAddr(pluginPort)

	for _, net := range u.networkings {
		config := sdk.IPDevConfig{
			Device: net.Device,
			IPCIDR: fmt.Sprintf("%s/%d", net.IP.String(), net.Prefix),
		}

		err := sdk.CreateIP(addr, config)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u *unit) removeNetworking() error {
	addr := u.getPluginAddr(pluginPort)

	for _, net := range u.networkings {
		config := sdk.IPDevConfig{
			Device: net.Device,
			IPCIDR: fmt.Sprintf("%s/%d", net.IP.String(), net.Prefix),
		}

		if err := sdk.RemoveIP(addr, config); err != nil {
			return err
		}
	}

	return nil
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
	_, err := u.getEngine()
	if err != nil {
		return err
	}
	address := ""
	if u.engine != nil {
		address = fmt.Sprintf("%s:%d", u.engine.IP, config.ConsulPort)
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
		swm := context.getSwithManagerUnit()
		if swm != nil {
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

func (u *unit) DeregisterHealthCheck(config consulapi.Config) error {
	eng, err := u.getEngine()
	if err != nil {
		return err
	}
	parts := strings.SplitN(config.Address, ":", 2)
	if len(parts) == 1 {
		parts[1] = parts[0]
	}

	config.Address = fmt.Sprintf("%s:%s", eng.IP, parts[1])

	client, err := consulapi.NewClient(&config)
	if err != nil {
		return err
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
		Mode:      "0600",
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

func (gd *Gardener) StartUnitService(NameOrID string) error {
	unit, err := database.GetUnit(NameOrID)
	if err != nil {
		err = fmt.Errorf("Not Found unit %s,%s", NameOrID, err)
		return err
	}

	u, err := gd.GetUnit(unit)
	if err != nil {
		return err
	}

	if err = u.startContainer(); err != nil {
		return err
	}

	return u.startService()
}

func (gd *Gardener) StopUnitService(NameOrID string, timeout int) error {
	unit, err := database.GetUnit(NameOrID)
	if err != nil {
		err = fmt.Errorf("Not Found unit %s,%s", NameOrID, err)
		return err
	}

	u, err := gd.GetUnit(unit)
	if err != nil {
		return err
	}

	return u.stopContainer(timeout)
}

func (u *unit) startService() error {
	if u.engine == nil {
		return errEngineIsNil
	}
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

func (u *unit) registerHorus(user, password string, agentPort int) (registerService, error) {
	node, err := database.GetNode(u.EngineID)
	if err != nil {
		return registerService{}, err
	}

	typ := u.Type
	switch u.Type {
	case _SwitchManagerType, "switch manager", "switchmanager":
		typ = "swm"
	case _ProxyType, "upproxy":
		typ = "upproxy"
	case _UpsqlType:
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
