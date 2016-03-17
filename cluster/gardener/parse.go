package gardener

import (
	"strconv"
	"strings"

	"github.com/docker/swarm/cluster"
	"github.com/samalba/dockerclient"
)

func defaultContainerConfig() *cluster.ContainerConfig {
	return &cluster.ContainerConfig{

		ContainerConfig: dockerclient.ContainerConfig{
			AttachStdout: true,
			AttachStderr: true,
			ExposedPorts: make(map[string]struct{}),
			Env:          make([]string, 0, 5),
			Cmd:          make([]string, 0, 5),
			Volumes:      make(map[string]struct{}),
			Labels:       make(map[string]string),

			HostConfig: dockerclient.HostConfig{
				Binds:         make([]string, 0, 5),
				NetworkMode:   "default",
				RestartPolicy: dockerclient.RestartPolicy{Name: "no"},
			},
			NetworkingConfig: dockerclient.NetworkingConfig{},
		},
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
		config.ExposedPorts = make(map[string]struct{})
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

	if config.HostConfig.RestartPolicy == (dockerclient.RestartPolicy{}) {

		config.HostConfig.RestartPolicy = dockerclient.RestartPolicy{
			Name:              "no",
			MaximumRetryCount: 0,
		}
	}

	return config
}

// "c;c;c;":3
func getCPU_Num(s string) int {
	num := 1
	for _, char := range s {
		if char == ';' {
			num++
		}
	}

	return num
}

// "master:1--standby:1--slave:3"
func getNodeArch(arch string) map[string]int {
	s := strings.Split(arch, "--")
	out := make(map[string]int)
	for i := range s {
		parts := strings.Split(s[i], ":")
		if len(parts) == 2 {

			if n, err := strconv.Atoi(parts[1]); err == nil {
				out[parts[0]] = n
			}
		}
	}

	return out
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
