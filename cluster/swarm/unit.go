package swarm

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/astaxie/beego/config"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/cluster/swarm/storage"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var (
	errEngineIsNil    = errors.New("Engine is nil")
	errEngineAPIisNil = errors.New("Engine API client is nil")
)

// ContainerCmd commands of actions
type containerCmd interface {
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
	defaultUserConfig(args ...interface{}) (map[string]interface{}, error)
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
	configures  map[string]interface{}

	configParser
	containerCmd
}

func (u *unit) factory() error {
	name, version := "", ""
	parts := strings.SplitN(u.ImageName, ":", 2)
	if len(parts) == 2 {
		name, version = parts[0], parts[1]
	} else {
		image, err := database.GetImageByID(u.ImageID)
		if err != nil {
			return err
		}

		name, version = image.Name, image.Version
	}

	parser, cmder, err := initialize(name, version)
	if err != nil {
		logrus.WithField("Unit", u.Name).Error(err)
	}

	if u.parent != nil && parser != nil {
		_, err := parser.ParseData([]byte(u.parent.Content))
		if err != nil {
			logrus.WithField("Unit", u.Name).WithError(err).Errorf("Parser unitConfig content")
		}
	}

	u.configParser = parser
	u.containerCmd = cmder

	return nil
}

func (u *unit) Engine() (*cluster.Engine, error) {
	if u.engine != nil {
		return u.engine, nil
	}

	if u.container != nil && u.container.Engine != nil {
		u.engine = u.container.Engine

		return u.engine, nil
	}

	return nil, errors.Wrapf(errEngineIsNil, "get %s Engine", u.Name)
}

func (u *unit) containerAPIClient() (*cluster.Engine, client.ContainerAPIClient, error) {
	eng, err := u.Engine()
	if err != nil {
		return nil, nil, err
	}

	if eng == nil {
		return nil, nil, errors.Wrap(errEngineIsNil, "get container API Client")
	}

	client := eng.ContainerAPIClient()
	if client == nil {
		return eng, nil, errors.Wrap(errEngineAPIisNil, "get container API Client")
	}

	return eng, client, nil
}

//func (gd *Gardener) GetUnit(table database.Unit) (*unit, error) {
//	var (
//		svc *Service
//		u   *unit
//	)
//	gd.RLock()
//	for i := range gd.services {
//		if gd.services[i].ID == table.ServiceID {
//			svc = gd.services[i]
//			break
//		}
//	}
//	gd.RUnlock()

//	if svc != nil {
//		svc.RLock()
//		u, _ = svc.getUnit(table.ID)
//		svc.RUnlock()
//	}

//	if u == nil || u.engine == nil {
//		value, err := gd.rebuildUnit(table)
//		if err != nil {
//			logrus.WithFields(logrus.Fields{
//				"Service": svc.Name,
//				"Unit":    table.Name,
//			}).Errorf("rebuild Unit:%s", err)

//			return nil, err
//		}

//		u = &value
//	}

//	return u, nil
//}

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

	entry := logrus.WithField("Unit", u.Name)

	if u.engine == nil && u.EngineID != "" {
		_, node, err := gd.getNode(u.EngineID)
		if err != nil {
			entry.WithError(err).Error("not found engine:", u.EngineID)

		} else if node != nil && node.engine != nil {
			u.engine = node.engine
		}
	}

	if u.ConfigID != "" {
		config, err := database.GetUnitConfigByID(u.ConfigID)
		if err != nil {
			entry.WithError(err).Errorf("get UnitConfig by ConfigID")
		}
		u.parent = config
	}

	ports, err := database.ListPortsByUnit(u.ID)
	if err == nil {
		u.ports = ports
	} else {
		entry.WithError(err).Error("get ports by unitID")
	}

	u.networkings, err = u.getNetworkings()
	if err != nil {
		entry.WithError(err).Error("get Unit networkings")
	}

	err = u.factory()

	entry.Debugf("rebuild Unit:%v", err)

	return u, err
}

func (u *unit) getNetworkings() ([]IPInfo, error) {
	if len(u.networkings) > 0 {
		return u.networkings, nil
	}
	networkings, err := getIPInfoByUnitID(u.ID, u.engine)
	if err != nil {
		return nil, err
	}

	u.networkings = networkings

	return u.networkings, nil
}

func (u *unit) getNetworkingAddr(networking, portName string) (string, int, error) {
	var addr string

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

	return "", 0, errors.Errorf("not found required networking='%s' port='%s'", networking, portName)
}

func pullImage(engine *cluster.Engine, image string, authConfig *types.AuthConfig) error {
	if engine == nil {
		return errEngineIsNil
	}
	if image == "" {
		return fmt.Errorf("params error,image:%s", image)
	}

	logrus.Debugf("Engine %s Pull image:%s", engine.Addr, image)

	err := engine.Pull(image, authConfig)
	if err != nil {
		// try again
		err := engine.Pull(image, authConfig)
		if err != nil {
			return err
		}
	}

	if image1 := engine.Image(image); image1 == nil {
		return fmt.Errorf("Not Found Image %s On Engine %s", image, engine.Addr)
	}

	return nil
}

func createVolume(eng *cluster.Engine, lv database.LocalVolume) (*types.Volume, error) {
	logrus.Debugf("Engine %s create Volume %s", eng.Addr, lv.Name)

	req := &types.VolumeCreateRequest{
		Name:       lv.Name,
		Driver:     lv.Driver,
		DriverOpts: map[string]string{"size": strconv.Itoa(lv.Size), "fstype": lv.Filesystem, "vgname": lv.VGName},
		Labels:     nil,
	}
	v, err := eng.CreateVolume(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Engine %s Create Volume %v", eng.Addr, req)
	}

	return v, nil
}

func createSanStoreageVG(host, name string, lun []database.LUN) error {
	logrus.Debugf("Engine %s create San Storeage VG,name=%s", host, name)
	list := make([]database.LUN, 0, len(lun))
	for i := range lun {
		if lun[i].Name == name {
			list = append(list, lun[i])
		}
	}

	if len(list) == 0 {
		return nil
	}

	store, err := storage.GetStore(list[0].StorageSystemID)
	if err != nil {
		return err
	}

	l, size := make([]int, len(list)), 0

	for i := range list {
		l[i] = list[i].HostLunID
		size += list[i].SizeByte
	}

	config := sdk.VgConfig{
		HostLunID: l,
		VgName:    list[0].VGName,
		Type:      store.Vendor(),
	}

	addr := getPluginAddr(host, pluginPort)

	err = sdk.SanVgCreate(addr, config)
	if err != nil {
		return errors.Wrapf(err, "Create SAN VG ON %s", addr)
	}

	return nil
}

func extendSanStoreageVG(host string, lun database.LUN) error {
	store, err := storage.GetStore(lun.StorageSystemID)
	if err != nil {
		return err
	}

	config := sdk.VgConfig{
		HostLunID: []int{lun.HostLunID},
		VgName:    lun.VGName,
		Type:      store.Vendor(),
	}

	addr := getPluginAddr(host, pluginPort)

	return sdk.SanVgExtend(addr, config)
}

func (u *unit) updateContainer(updateConfig container.UpdateConfig) error {
	engine, client, err := u.containerAPIClient()
	if err != nil {
		return err
	}

	err = client.ContainerUpdate(context.Background(), u.container.ID, updateConfig)
	engine.CheckConnectionErr(err)

	return errors.Wrap(err, "container update")
}

func (u *unit) removeContainer(force, rmVolumes bool) error {
	engine, err := u.Engine()
	if err != nil {
		return err
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

	err = engine.RemoveContainer(c, force, rmVolumes)
	if err != nil {
		return err
	}

	err = removeNetworkings(engine.IP, u.networkings)

	return err
}

func (u *unit) startContainer() error {
	engine, err := u.Engine()
	if err != nil {
		return err
	}

	networkings, err := u.getNetworkings()
	if err != nil {
		return err
	}

	return startContainer(u.ContainerID, engine, networkings)
}

func startContainer(containerID string, engine *cluster.Engine, networkings []IPInfo) error {
	if engine == nil {
		return errors.Wrap(errEngineIsNil, "start contianer "+containerID)
	}

	err := createNetworking(engine.IP, networkings)
	if err != nil {
		return err
	}

	err = engine.StartContainer(containerID, nil)
	engine.CheckConnectionErr(err)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Engine":    engine.Addr,
			"Container": containerID,
		}).WithError(err).Error("start container")

		return errors.Wrapf(err, "unable to start container %s on engine %s", containerID, engine.Addr)
	}

	return nil
}

func (u *unit) forceStopContainer(timeout int) error {
	engine, client, err := u.containerAPIClient()
	if err != nil {
		return err
	}

	var timeoutptr *time.Duration
	if timeout > 0 {
		temp := time.Duration(timeout) * time.Second
		timeoutptr = &temp
	}

	err = client.ContainerStop(context.Background(), u.Unit.ContainerID, timeoutptr)
	engine.CheckConnectionErr(err)

	if err = checkContainerError(err); err == errContainerNotRunning {
		return nil
	}

	return errors.Wrap(err, "container stop")
}

func (u *unit) stopContainer(timeout int) error {
	if val := atomic.LoadInt64(&u.Status); val == statusUnitBackuping {
		return errors.Errorf("Unit %s is Backuping,Cannot stop", u.Name)
	}

	return u.forceStopContainer(timeout)
}

func (u *unit) restartContainer(ctx context.Context) error {
	if u.containerCmd == nil {
		return nil
	}

	engine, client, err := u.containerAPIClient()
	if err != nil {
		return err
	}

	cmd := u.StopServiceCmd()
	if len(cmd) == 0 {
		logrus.WithField("Unit", u.Name).Warn("StopServiceCmd is nil")
		return nil
	}

	inspect, err := containerExec(ctx, engine, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = errors.Errorf("%s stop service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	timeout := 5 * time.Second

	err = client.ContainerRestart(ctx, u.Unit.ContainerID, &timeout)
	engine.CheckConnectionErr(err)

	return errors.Wrap(err, "container restart")
}

func (u *unit) renameContainer(name string) error {
	engine, client, err := u.containerAPIClient()
	if err != nil {
		return err
	}

	err = client.ContainerRename(context.Background(), u.container.ID, name)
	engine.CheckConnectionErr(err)

	return errors.Wrap(err, "container rename")
}

func (u *unit) kill() error {
	_, client, err := u.containerAPIClient()
	if err != nil {
		return err
	}

	database.TxUpdateUnitStatus(&u.Unit, statusUnitDeleting, "")

	return client.ContainerKill(context.Background(), u.ContainerID, "KILL")
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

func removeNetworkings(host string, networkings []IPInfo) error {
	addr := getPluginAddr(host, pluginPort)

	for _, net := range networkings {
		config := sdk.IPDevConfig{
			Device: net.Device,
			IPCIDR: fmt.Sprintf("%s/%d", net.IP.String(), net.Prefix),
		}

		err := sdk.RemoveIP(addr, config)
		if err != nil {
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

func (u *unit) activateVG(config sdk.ActiveConfig) error {
	engine, err := u.Engine()
	if err != nil {
		return err
	}

	addr := getPluginAddr(engine.IP, pluginPort)
	err = sdk.SanActivate(addr, config)
	if err != nil {
		logrus.Errorf("%s SanDeActivate error:%s", u.Name, err)
	}
	return nil
}

func (u *unit) deactivateVG(config sdk.DeactivateConfig) error {
	engine, err := u.Engine()
	if err != nil {
		return err
	}

	addr := getPluginAddr(engine.IP, pluginPort)
	err = sdk.SanDeActivate(addr, config)
	if err != nil {
		logrus.Errorf("%s SanDeActivate error:%s", u.Name, err)
	}

	return err
}

func (u *unit) copyConfig(data map[string]interface{}) error {
	_, err := u.ParseData([]byte(u.parent.Content))
	if err != nil {
		return err
	}

	err = u.verify(data)
	if err != nil {
		return err
	}

	for key, val := range data {

		err := u.Set(key, val)
		if err != nil {
			return err
		}
	}

	content, err := u.Marshal()
	if err != nil {
		return err
	}

	err = u.saveConfigToDisk(content)
	if err != nil {
		return err
	}

	context := string(content)

	volumes, err := database.ListVolumesByUnitID(u.ID)
	if err != nil {
		return err
	}

	engine, err := u.Engine()
	if err != nil {
		return err
	}

	err = copyConfigIntoCNFVolume(engine.IP, u.Path(), context, volumes)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"Unit":  u.Name,
			"Image": u.ImageName,
		}).WithError(err).Error("copy file to Volome")
	}

	return err
}

func copyConfigIntoCNFVolume(host, path, content string, lvs []database.LocalVolume) error {
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

	addr := getPluginAddr(host, pluginPort)
	err := sdk.CopyFileToVolume(addr, config)

	logrus.WithFields(logrus.Fields{
		"addr":   addr,
		"volume": lvs[cnf].Name,
	}).WithError(err).Debugf("CopyFileToVolume,config:+v", config)

	return errors.Wrap(err, "copy file to Volume")
}

func (u *unit) initService() error {
	if u.containerCmd == nil {
		return nil
	}
	cmd := u.InitServiceCmd()
	if len(cmd) == 0 {
		logrus.WithField("Unit", u.Name).Warn(" InitServiceCmd is nil")
		return nil
	}

	inspect, err := containerExec(context.Background(), u.engine, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = errors.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	if err == nil {
		atomic.StoreInt64(&u.Status, statusUnitStarted)
	} else {
		atomic.StoreInt64(&u.Status, statusUnitStartFailed)
		u.LatestError = err.Error()
	}

	return err
}

func initUnitService(containerID string, eng *cluster.Engine, cmd []string) error {
	if len(cmd) == 0 {
		logrus.WithField("Container", containerID).Warn("InitServiceCmd is nil")
		return nil
	}

	inspect, err := containerExec(context.Background(), eng, containerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = errors.Errorf("%s init service cmd:%s exitCode:%d,%v,Error:%v", containerID, cmd, inspect.ExitCode, inspect, err)
	}

	return err
}

// StartUnitService start the unit service
func (gd *Gardener) StartUnitService(nameOrID string) error {
	unit, err := database.GetUnit(nameOrID)
	if err != nil {
		return err
	}

	svc, err := gd.GetService(unit.ServiceID)
	if err != nil {
		return err
	}

	svc.Lock()
	defer svc.Unlock()

	u, err := svc.getUnit(unit.ID)
	if err != nil {
		return err
	}

	err = u.startContainer()
	if err != nil {
		return err
	}

	return u.startService()
}

// StopUnitService stop the unit service & container
func (gd *Gardener) StopUnitService(nameOrID string, timeout int) error {
	unit, err := database.GetUnit(nameOrID)
	if err != nil {
		return errors.Wrap(err, "not found Unit:"+nameOrID)
	}

	svc, err := gd.GetService(unit.ServiceID)
	if err != nil {
		return err
	}

	svc.Lock()
	defer svc.Unlock()

	u, err := svc.getUnit(unit.ID)
	if err != nil {
		return err
	}

	err = u.stopService()
	if err != nil {
		return err
	}

	return u.stopContainer(timeout)
}

func (u *unit) startService() (err error) {
	defer func() {
		code, msg := int64(statusUnitStarted), ""
		if err != nil {
			code, msg = statusUnitStartFailed, err.Error()
		}

		_err := database.TxUpdateUnitStatus(&u.Unit, code, msg)

		if _err != nil {
			logrus.WithFields(logrus.Fields{
				"Unit":   u.Name,
				"Status": code,
				"Error":  msg,
			}).WithError(_err).Error("Update Unit status")
		}
	}()

	err = u.StatusCAS("!=", statusUnitBackuping, statusUnitStarting)
	if err != nil {
		logrus.WithError(err).Errorf("Start %s service", u.Name)

		return err
	}

	eng, err := u.Engine()
	if err != nil {
		return err
	}

	if u.containerCmd == nil {
		return nil
	}

	cmd := u.StartServiceCmd()
	if len(cmd) == 0 {
		logrus.WithField("Unit", u.Name).Warn("StartServiceCmd is nil")
		return nil
	}

	inspect, err := containerExec(context.Background(), eng, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = errors.Errorf("%s start service cmd=%s exitCode=%d,%v,Error:%+v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	return err
}

func (u *unit) forceStopService() error {
	eng, err := u.Engine()
	if err != nil {
		return err
	}

	if u.containerCmd == nil {
		return nil
	}
	cmd := u.StopServiceCmd()
	if len(cmd) == 0 {
		logrus.WithField("Unit", u.Name).Warn("StopServiceCmd is nil")
		return nil
	}

	inspect, err := containerExec(context.Background(), eng, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = errors.Errorf("%s stop service cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	return err
}

func (u *unit) stopService() error {
	err := u.StatusCAS("!=", statusUnitBackuping, statusUnitStoping)
	if err != nil {
		logrus.WithError(err).Errorf("Start %s service", u.Name)

		return err
	}

	code, msg := int64(statusUnitStoping), ""
	_err := database.TxUpdateUnitStatus(&u.Unit, code, msg)
	if _err != nil {
		logrus.WithFields(logrus.Fields{
			"Unit":    u.Name,
			"message": msg,
			"status":  code,
		}).WithError(_err).Error("Update Unit status")
	}
	err = u.forceStopService()

	code = statusUnitStoped

	if err != nil {
		code, msg = statusUnitStopFailed, err.Error()
	}

	_err = database.TxUpdateUnitStatus(&u.Unit, code, msg)
	if _err != nil {
		logrus.WithFields(logrus.Fields{
			"Unit":    u.Name,
			"status":  code,
			"message": msg,
		}).WithError(_err).Error("Update Unit status")
	}

	return err
}

func (u *unit) backup(ctx context.Context, args ...string) (err error) {
	if u.container.State != "running" {
		return errors.Errorf("Unit %s is busy,container Status=%s", u.Name, u.container.State)
	}

	err = u.StatusCAS("!=", statusUnitStoping, statusUnitBackuping)
	if err != nil {
		return err
	}

	defer func() {
		if err == nil {
			u.Status = statusUnitBackuped
			u.LatestError = ""
		} else {
			u.Status = statusUnitBackupFailed
			u.LatestError = err.Error()
		}
	}()

	eng, err := u.Engine()
	if err != nil {
		return err
	}

	if u.containerCmd == nil {
		return nil
	}
	cmd := u.BackupCmd(args...)
	if len(cmd) == 0 {
		logrus.WithField("Unit", u.Name).Warn("BackupCmd is nil")
		return nil
	}

	inspect, err := containerExec(ctx, eng, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = errors.Errorf("%s backup cmd:%s exitCode:%d,%v,Error:%+v", u.Name, cmd, inspect.ExitCode, inspect, err)
	}

	return err
}

func (u *unit) restore(ctx context.Context, file string) error {
	eng, err := u.Engine()
	if err != nil {
		return err
	}

	if u.containerCmd == nil {
		return nil
	}

	cmd := u.RestoreCmd(file)
	if len(cmd) == 0 {
		logrus.WithField("Unit", u.Name).Warn("RestoreCmd is nil")
		return nil
	}

	inspect, err := containerExec(ctx, eng, u.ContainerID, cmd, false)
	if inspect.ExitCode != 0 {
		err = errors.Errorf("%s restore cmd:%s exitCode:%d,%v,Error:%v", u.Name, cmd, inspect.ExitCode, inspect, err)
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
	err := database.UpdateUnitInfo(u.Unit)
	if err != nil {
		logrus.WithField("Container", u.Name).WithError(err).Errorf("Update Unit Info:%+v", u.Unit)
	}

	return err
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

// RestoreUnit restore unit volume data from backupfile
func (gd *Gardener) RestoreUnit(nameOrID, source string) (string, error) {
	table, err := database.GetUnit(nameOrID)
	if err != nil {
		return "", err
	}

	service, err := gd.GetService(table.ServiceID)
	if err != nil {
		return "", err
	}

	service.RLock()
	unit, err := service.getUnit(table.ID)
	service.RUnlock()

	if err != nil {
		return "", err
	}

	background := func(ctx context.Context) error {
		// restart container
		err = unit.restartContainer(ctx)
		if err != nil {
			return err
		}

		unit.Status, unit.LatestError = statusUnitRestored, ""

		err = unit.restore(ctx, source)
		if err != nil {
			err = errors.Wrapf(err, "Unit %s restore", unit.Name)
			logrus.Error(err)

			unit.Status, unit.LatestError = statusUnitRestoreFailed, err.Error()
		}

		return err
	}

	task := database.NewTask(unit.Name, unitRestoreTask, unit.ID, "", nil, 0)

	before := func() error {
		err := unit.StatusCAS("!=", statusUnitBackuping, statusUnitRestoring)
		if err != nil {
			logrus.WithError(err).Errorf("Start %s service", unit.Name)

			return err
		}

		return task.Insert()
	}

	update := func(code int, msg string) error {
		task.Status = int64(code)

		return database.TxUpdateUnitStatusWithTask(&unit.Unit, &task, msg)
	}

	t := NewAsyncTask(context.Background(),
		background,
		before,
		update,
		0)

	return task.ID, t.Run()
}
