package swarm

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/astaxie/beego/config"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/store"
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
	RestoreCmd(file string) []string
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
		_, node, err := gd.GetNode(u.EngineID)
		if err != nil {
			logrus.Errorf("Not Found Node %s,Error:%s", u.EngineID, err)
		} else if node != nil && node.engine != nil {
			u.engine = node.engine
		}
	}

	if u.ConfigID != "" {
		config, err := database.GetUnitConfigByID(u.ConfigID)
		if err == nil {
			u.parent = config
		} else {
			logrus.Errorf("Cannot Query unit Parent Config By ConfigID %s,Error:%s", u.ConfigID, err)
		}

	}

	ports, err := database.ListPortsByUnit(u.ID)
	if err == nil {
		u.ports = ports
	} else {
		logrus.Errorf("Cannot Query unit ports By UnitID %s,Error:%s", u.ID, err)
	}

	u.networkings, err = u.getNetworkings()
	if err != nil {
		logrus.Errorf("Cannot Query unit networkings By UnitID %s,Error:%s", u.ID, err)
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
	// prepare for volumes
	lvs, err := database.SelectVolumesByUnitID(u.ID)
	if err != nil {
		logrus.Error("SelectVolumesByUnitID %s error,%s", u.ID, err)
		return err
	}
	for i := range lvs {
		// if volume create on san storage,should created VG before create Volume
		if strings.Contains(lvs[i].VGName, "_SAN_VG") {
			err := createSanStoreageVG(u.engine.IP, lvs[i].Name)
			if err != nil {
				return err
			}
		}

		_, err = createVolume(u.engine, lvs[i])
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

func createVolume(eng *cluster.Engine, lv database.LocalVolume) (*cluster.Volume, error) {
	req := &types.VolumeCreateRequest{
		Name:       lv.Name,
		Driver:     lv.Driver,
		DriverOpts: map[string]string{"size": strconv.Itoa(lv.Size), "fstype": lv.Filesystem, "vgname": lv.VGName},
		Labels:     nil,
	}
	v, err := eng.CreateVolume(req)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func createSanStoreageVG(host, lun string) error {
	list, err := database.ListLUNByName(lun)
	if err != nil {
		return fmt.Errorf("LUN:%s,ListLUNByName Error:%s", lun, err)
	}
	if len(list) == 0 {
		return nil
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

	addr := getPluginAddr(host, pluginPort)

	return sdk.SanVgCreate(addr, config)
}

func extendSanStoreageVG(host, lunID string) error {
	lun, err := database.GetLUNByID(lunID)
	if err != nil {
		return fmt.Errorf("LUN:%s,ListLUNByName Error:%s", lunID, err)
	}

	storage, err := store.GetStoreByID(lun.StorageSystemID)
	if err != nil {
		return err
	}

	config := sdk.VgConfig{
		HostLunId: []int{lun.HostLunID},
		VgName:    lun.VGName,
		Type:      storage.Vendor(),
	}

	addr := getPluginAddr(host, pluginPort)

	return sdk.SanVgExtend(addr, config)
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

	return client.ContainerUpdate(context.Background(), u.container.ID, updateConfig)
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

	err = removeNetworking(engine.IP, u.networkings)

	return err
}

func (u *unit) startContainer() error {
	if u.engine == nil {
		return errEngineIsNil
	}

	networkings, err := u.getNetworkings()
	if err != nil {
		logrus.Errorf("Cannot Query unit networkings By UnitID %s,Error:%s", u.ID, err)

		return err
	}

	err = createNetworking(u.engine.IP, networkings)
	if err != nil {
		logrus.Errorf("%s create Networking error:%s,networkings:%v", u.Name, err, networkings)
		return err
	}

	return u.engine.StartContainer(u.Unit.Name, nil)
}

func (u *unit) stopContainer(timeout int) error {
	u.stopService()

	client, err := u.getEngineAPIClient()
	if err != nil {
		return err
	}

	err = client.ContainerStop(context.Background(), u.Unit.ContainerID, timeout)
	if err := checkContainerError(err); err == errContainerNotRunning {
		return nil
	}

	return err
}

func (u *unit) restartContainer(timeout int) error {
	err := u.stopService()
	if err != nil {
		// return err
	}

	if u.engine == nil {
		return errEngineIsNil
	}

	client := u.engine.EngineAPIClient()
	if client == nil {
		return errEngineAPIisNil
	}

	return client.ContainerRestart(context.Background(), u.Unit.ContainerID, timeout)
}

func (u *unit) renameContainer(name string) error {
	client, err := u.getEngineAPIClient()
	if err != nil {
		return err
	}

	return client.ContainerRename(context.Background(), u.container.ID, u.Unit.ID)
}

func createNetworking(host string, networkings []IPInfo) error {
	addr := getPluginAddr(host, pluginPort)

	for _, net := range networkings {
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

func removeNetworking(host string, networkings []IPInfo) error {
	addr := getPluginAddr(host, pluginPort)

	for _, net := range networkings {
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

func updateVolume(host string, lv database.LocalVolume, size int) error {
	option := sdk.VolumeUpdateOption{
		VgName: lv.VGName,
		LvName: lv.Name,
		FsType: lv.Filesystem,
		Size:   lv.Size,
	}

	addr := getPluginAddr(host, pluginPort)
	err := sdk.VolumeUpdate(addr, option)
	if err != nil {
		logrus.Errorf("host:%s volume update error:%s", addr, err)
	}

	return err
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

	context := string(content)

	logrus.Debugf("default:%s\ndefaultUser:%v\nconfig:%s", u.parent.Content, data, context)

	volumes, err := database.SelectVolumesByUnitID(u.ID)
	if err != nil {
		return err
	}

	err = copyConfigIntoCNFVolume(u, volumes, context)
	if err != nil {
		logrus.Errorf("copyConfigIntoCNFVolume error:%s", err)
		return err
	}

	return nil
}

func copyConfigIntoCNFVolume(u *unit, lvs []database.LocalVolume, content string) error {
	cnf := 0
	for i := range lvs {
		if strings.Contains(lvs[i].Name, "_CNF_LV") {
			cnf = i
			break
		}
		if strings.Contains(lvs[i].Name, "_DAT_LV") {
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
		VgName:    lvs[cnf].VGName,
		LvsName:   lvs[cnf].Name,
		MountName: lvs[cnf].Name,
		Data:      content,
		FDes:      path,
		Mode:      "0600",
	}

	addr := getPluginAddr(u.engine.IP, pluginPort)
	err := sdk.FileCopyToVolome(addr, config)

	logrus.Debugf("FileCopyToVolome to %s:%s config:%v", addr, lvs[cnf].Name, config)

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

	return err
}

func initService(id string, eng *cluster.Engine, cmd []string) error {
	if len(cmd) == 0 {
		logrus.Warnf("%s InitServiceCmd is nil", id)
		return nil
	}

	inspect, err := containerExec(eng, id, cmd, false)
	if inspect.ExitCode != 0 {
		err = fmt.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", id, cmd, inspect.ExitCode, inspect, err)
	}

	return err
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
		err = fmt.Errorf("%s start service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	return err
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
		err = fmt.Errorf("%s stop service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	return err
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
		err = fmt.Errorf("%s backup cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	return err
}

func (u *unit) restore(file string) error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.RestoreCmd(file)
	if len(cmd) == 0 {
		logrus.Warnf("%s RestoreCmd is nil", u.Name)
		return nil
	}
	logrus.WithFields(logrus.Fields{
		"Name": u.Name,
		"Cmd":  cmd,
	}).Debugln("restore job")

	inspect, err := containerExec(u.engine, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = fmt.Errorf("%s restore cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	return err

}

var pluginPort = 3333

func getPluginAddr(IP string, port int) string {
	if port == 0 {
		port = pluginPort
	}

	return fmt.Sprintf("%s:%d", IP, port)
}

func (u *unit) saveToDisk() error {

	return database.UpdateUnitInfo(u.Unit)
}

func (u *unit) registerHorus(user, password string, agentPort int) (registerService, error) {
	node, err := database.GetNode(u.EngineID)
	if err != nil {
		return registerService{}, err
	}

	_type := u.Type
	switch u.Type {
	case _SwitchManagerType, "switch manager", "switchmanager":
		_type = "swm"
	case _ProxyType, "upproxy":
		_type = "upproxy"
	case _UpsqlType:
		_type = "upsql"
	default:
		return registerService{}, fmt.Errorf("Unsupported Type:'%s'", u.Type)
	}

	return registerService{
		Endpoint:      u.ID,
		CollectorName: u.Name,
		User:          user,
		Password:      password,
		Type:          _type,
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

func (gd *Gardener) RestoreUnit(NameOrID, source string) error {
	table, err := database.GetUnit(NameOrID)
	if err != nil {
		return err
	}

	service, err := gd.GetService(table.ServiceID)
	if err != nil {
		return err
	}

	unit, err := service.getUnit(table.ID)
	if err != nil {
		return err
	}

	// manager locked
	// restart container
	err = unit.restartContainer(5)
	if err != nil {
		return err
	}

	err = unit.restore(source)

	return err
}
