package swarm

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
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

type configParser interface {
	Validate(data map[string]interface{}) error
	ParseData(data []byte) (config.Configer, error)
	defaultUserConfig(svc *Service, u *unit) (map[string]interface{}, error)
	Marshal() ([]byte, error)
	PortSlice() (bool, []port)
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

		//savetoDisk
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

func (u *unit) RegisterHealthCheck(client *consulapi.Client) error {
	if client == nil {
		return fmt.Errorf("consul client is nil")
	}
	agent := client.Agent()
	service := consulapi.AgentServiceRegistration{
		ID:      "",
		Name:    "",
		Tags:    []string{},
		Port:    0,
		Address: "",
		Check: &consulapi.AgentServiceCheck{
			Script:            "",
			DockerContainerID: "",
			Shell:             "",
			Interval:          "",
			Timeout:           "",
		},
	}

	return agent.ServiceRegister(&service)
}

func (u *unit) DeregisterHealthCheck(client *consulapi.Client) error {

	return client.Agent().ServiceDeregister("")
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

	config := sdk.VolumeFileConfig{
		VgName:    volumes[cnf].VGName,
		LvsName:   volumes[cnf].Name,
		MountName: volumes[cnf].Name,
		Data:      string(content),
		FDes:      u.Path(),
		Mode:      "0666",
	}

	log.Debugf("default:%s\ndefaultUser:%v\nconfig:%s", u.parent.Content, data, config.Data)

	log.Debugf("VolumeFileConfig:%+v", config)
	err = sdk.FileCopyToVolome(u.getPluginAddr(pluginPort), config)

	return err
}

func (u *unit) initService() error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.InitServiceCmd()

	return containerExec(u.engine, u.ContainerID, cmd, false)
}

func (u *unit) startService() error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.StartServiceCmd()

	return containerExec(u.engine, u.ContainerID, cmd, false)
}

func (u *unit) stopService() error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.StopServiceCmd()

	return containerExec(u.engine, u.ContainerID, cmd, false)
}

func (u *unit) backup(args ...string) error {
	if u.ContainerCmd == nil {
		return nil
	}
	cmd := u.BackupCmd(args...)

	log.WithFields(log.Fields{
		"Name": u.Name,
		"Cmd":  cmd,
	}).Debugln("start Backup job")

	return containerExec(u.engine, u.ContainerID, cmd, false)
}

// containerExec
func containerExec(engine *cluster.Engine, containerID string, cmd []string, detach bool) error {
	client := engine.EngineAPIClient()
	if client == nil {
		return errors.New("Engine APIClient is nil")
	}

	execConfig := types.ExecConfig{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          cmd,
		Container:    containerID,
		Detach:       detach,
	}

	if detach {
		execConfig.AttachStderr = false
		execConfig.AttachStdout = false
	}

	resp, err := client.ContainerExecCreate(context.TODO(), execConfig)
	if err != nil {
		return err
	}

	return client.ContainerExecStart(context.TODO(), resp.ID, types.ExecStartCheck{Detach: detach})
}

var pluginPort = 3333

func (u unit) getPluginAddr(port int) string {

	return fmt.Sprintf("%s:%d", u.engine.IP, port)
}

func (u *unit) saveToDisk() error {
	return nil
}

func (u *unit) registerToHorus(addr, user, password string, agentPort int) error {
	if u.engine == nil {
		return errors.New("Engine is nil")
	}
	obj := registerService{
		Endpoint:      u.ID,
		CollectorName: u.Name,
		User:          user,
		Password:      password,
		Type:          u.Type,
		CollectorIP:   u.engine.IP,
		CollectorPort: agentPort,
		MetricTags:    u.NodeID,
		Network:       nil,
		Status:        "on",
		Table:         "host",
		CheckType:     "health",
	}

	return registerToHorus(addr, obj)
}
