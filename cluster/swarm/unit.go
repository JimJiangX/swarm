package swarm

import (
	"errors"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/agent"
	"github.com/docker/swarm/cluster/swarm/database"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/samalba/dockerclient"
	"golang.org/x/net/context"
)

type ContainerCmd interface {
	StartContainerCmd() []string
	StartServiceCmd() []string
	StopServiceCmd() []string
	RecoverCmd(file string) []string
	BackupCmd(args ...string) []string
	CleanBackupFileCmd(args ...string) []string
}

type configParser interface {
	Validate(data map[string]interface{}) error
	Parse(data string) (map[string]interface{}, error)
	Marshal(map[string]interface{}) ([]byte, error)
}

type unit struct {
	database.Unit
	engine      *cluster.Engine
	config      *cluster.ContainerConfig
	container   *cluster.Container
	parent      *database.UnitConfig
	ports       []database.Port
	networkings []IPInfo

	content map[string]interface{}

	configParser
	ContainerCmd
}

func (u *unit) factory() error {
	switch u.Type {
	case "mysql":
		u.configParser = &mysqlConfig{}
		// cmd
		u.ContainerCmd = &mysqlCmd{}

	case "proxy", "upproxy":
		u.configParser = &proxyConfig{}

		u.ContainerCmd = &proxyCmd{}

	case "switch manager", "SM":
		u.configParser = &switchManagerConfig{}

		u.ContainerCmd = &switchManagerCmd{}

	default:

		return fmt.Errorf("Unsupported Type:%s", u.Type)
	}

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

	return nil
}

func (u *unit) createContainer(authConfig *dockerclient.AuthConfig) (*cluster.Container, error) {
	container, err := u.engine.Create(u.config, u.Unit.ID, true, authConfig)
	if err == nil && container != nil {
		u.container = container
		u.Unit.ContainerID = container.Id

		//savetoDisk
	}

	return container, err
}

func (u *unit) updateContainer(updateConfig container.UpdateConfig) error {
	client := u.engine.EngineAPIClient()

	return client.ContainerUpdate(context.TODO(), u.container.Id, updateConfig)
}

func (u *unit) removeContainer(force, rmVolumes bool) error {
	err := u.engine.RemoveContainer(u.container, force, rmVolumes)
	if err != nil {
		err = u.engine.RemoveContainer(u.container, true, rmVolumes)
	}

	return err
}

func (u *unit) startContainer() error {
	return u.engine.StartContainer(u.Unit.ContainerID, nil)
}

func (u *unit) stopContainer(timeout int) error {
	err := u.stopService()
	if err != nil {
		return err
	}

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

	return client.ContainerRename(context.TODO(), u.container.Id, u.Unit.ID)
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
	agent := client.Agent()
	Service := consulapi.AgentServiceRegistration{
		ID:                "",
		Name:              "",
		Tags:              []string{},
		Port:              0,
		Address:           "",
		EnableTagOverride: false,
		Check: &consulapi.AgentServiceCheck{
			Script:            "",
			DockerContainerID: "",
			Shell:             "",
			Interval:          "",
			Timeout:           "",
		},
	}

	return agent.ServiceRegister(&Service)
}

func (u *unit) DeregisterHealthCheck(client *consulapi.Client) error {

	return client.Agent().ServiceDeregister("")
}

func (u *unit) Migrate(e *cluster.Engine, config *cluster.ContainerConfig) (*cluster.Container, error) {
	return nil, nil
}

func (u *unit) CopyConfig(data map[string]interface{}) error {
	err := u.Merge(data)
	if err != nil {
		return err
	}

	err = u.Verify(data)
	if err != nil {
		return err
	}

	content, err := u.Marshal(u.content)
	if err != nil {
		return err
	}

	if _, err = u.SaveToDisk(string(content)); err != nil {
		return err
	}

	config := sdk.VolumeFileConfig{
		VgName:    "",
		LvsName:   "",
		MountName: "",
		Data:      string(content),
		FDes:      u.Path(),
		Mode:      "0666",
	}

	err = sdk.FileCopyToVolome(u.getPluginAddr(pluginPort), config)

	return err
}

func (u *unit) startService() error {
	cmd := u.StartServiceCmd()

	return containerExec(u.engine, u.ContainerID, cmd, false)
}

func (u *unit) stopService() error {
	cmd := u.StopServiceCmd()

	return containerExec(u.engine, u.ContainerID, cmd, false)
}

func (u *unit) backup(args ...string) error {
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

func newVolumeCreateRequest(name, driver string, opts map[string]string) types.VolumeCreateRequest {
	if opts == nil {
		opts = make(map[string]string)
	}
	return types.VolumeCreateRequest{
		Name:       name,
		Driver:     driver,
		DriverOpts: opts,
	}
}

const pluginPort = 3333

func (u unit) getPluginAddr(port int) string {

	return fmt.Sprintf("%s:%d", u.engine.Addr, port)
}

func (u *unit) saveToDisk() error {
	return nil
}
