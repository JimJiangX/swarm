package swarm

import (
	"errors"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/docker/swarm/cluster"
)

func defaultContainerConfig() *cluster.ContainerConfig {
	return &cluster.ContainerConfig{
		Config: container.Config{
			AttachStdout: true,
			AttachStderr: true,
			ExposedPorts: make(map[nat.Port]struct{}),
			Env:          make([]string, 0, 5),
			Cmd:          make([]string, 0, 5),
			Volumes:      make(map[string]struct{}),
			Labels:       make(map[string]string),
		},
		HostConfig: container.HostConfig{
			Binds:         make([]string, 0, 5),
			NetworkMode:   "default",
			RestartPolicy: container.RestartPolicy{Name: "no"},
		},
		NetworkingConfig: network.NetworkingConfig{},
	}
}

func buildContainerConfig(config *cluster.ContainerConfig) *cluster.ContainerConfig {

	if config.AttachStdout == false {
		config.AttachStdout = true
	}

	if config.AttachStderr == false {
		config.AttachStderr = true
	}

	if config.ExposedPorts == nil {
		config.ExposedPorts = make(map[nat.Port]struct{})
	}

	if config.Cmd == nil {
		config.Cmd = make([]string, 0, 5)
	}

	if config.Env == nil {
		config.Env = make([]string, 0, 5)
	}

	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	if config.HostConfig.Binds == nil {
		config.HostConfig.Binds = make([]string, 0, 5)
	}

	if config.HostConfig.NetworkMode == "" {
		config.HostConfig.NetworkMode = "default"
	}

	if config.HostConfig.RestartPolicy == (container.RestartPolicy{}) {

		config.HostConfig.RestartPolicy = container.RestartPolicy{
			Name:              "no",
			MaximumRetryCount: 0,
		}
	}

	return config
}

func ParseCPUSets(val string) (map[int]bool, error) {
	return parsers.ParseUintList(val)
}

func validateContainerConfig(config *cluster.ContainerConfig) error {
	// validate config
	swarmID := config.SwarmID()
	if swarmID != "" {
		return errors.New("Swarm ID to the container have created")
	}

	if config.HostConfig.CPUShares != 0 {
		return errors.New("CPUShares > 0,CPUShares should be 0")
	}

	_, err := parseCpuset(config)
	if err != nil {
		return err
	}

	return nil
}

// parse NCPU from config.HostConfig.CpusetCpus
func parseCpuset(config *cluster.ContainerConfig) (int, error) {

	ncpu, err := strconv.Atoi(config.HostConfig.CpusetCpus)

	log.WithFields(log.Fields{
		"container ID": config.SwarmID(),
		"CpusetCpus":   config.HostConfig.CpusetCpus,
	}).Errorf("Parse CpusetCpus Error,%s", err)

	return ncpu, err
}

// "master:1--standby:1--slave:3"
func getServiceArch(arch string) (map[string]int, error) {
	s := strings.Split(arch, "--")
	out := make(map[string]int)
	for i := range s {
		parts := strings.Split(s[i], ":")
		if len(parts) == 2 {

			if n, err := strconv.Atoi(parts[1]); err == nil {
				out[parts[0]] = n
			} else {
				return nil, err
			}
		}
	}

	return out, nil
}

func parseKVuri(uri string) string {
	part := strings.SplitN(uri, "://", 2)
	if len(part) == 1 {
		uri = part[0]
	} else {
		uri = part[1]
	}

	part = strings.SplitN(uri, "/", 2)

	if len(part) == 2 {
		return part[1]
	}

	return ""
}

/*
type volume struct {
	types.Volume
	v              database.Volume
	FilesystemType string
	bind           string
}

func createVolume(name, typ, driver, mountpoint, bind,
	nodeID, vg, lunID string, size int64) *Volume {

	return &Volume{
		Volume: types.Volume{
			Name:       name,
			Driver:     driver,
			Mountpoint: mountpoint,
		},
		v: database.Volume{
			Name:     name,
			Type:     typ,
			SizeByte: size,
			NodeID:   nodeID,
			LunID:    lunID,
			VGName:   vg,
		},
		FilesystemType: "xfs",
		bind:           bind,
	}
}

const (
	LocalDisk    = "localdisk"
	defaultLocal = "local"
	NASDisk      = "NasDisk"
)

func parseVolumes(labels map[string]string) []*Volume {
	volumes := make([]*volume, 0, len(labels))

	for i := range v {
		var bind string
		if v[i].Name == "" {
			v[i].Name = utils.Generate16UUID()
		}
		if v[i].Mountpoint == "/DBAASDAT" {
			if v[i].Driver == "" {
				v[i].Driver = LocalDisk
			}
			v[i].Type = "-DAT"
			bind = "/" + v[i].Name + v[i].Type + ":" + v[i].Mountpoint

		} else if v[i].Mountpoint == "/DBAASLOG" {

			if v[i].Driver == "" {
				v[i].Driver = LocalDisk
			}
			v[i].Type = "-LOG"
			bind = "/" + v[i].Name + v[i].Type + ":" + v[i].Mountpoint

		} else if v[i].Mountpoint == "/BACKUP" {

			if v[i].Driver == "" {
				v[i].Driver = NASDisk
			}
			v[i].Type = "/NASBAK"

			bind = v[i].Type + "/" + v[i].Name + ":" + v[i].Mountpoint
		}

		volumes[i] = createVolume(v[i].Name, v[i].Type, v[i].Driver, bind,
			v[i].Mountpoint, node, "", "", int64(v[i].SizeMB)*MiB)
	}

	return volumes
}
*/
