package swarm

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/utils"
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
	return utils.ParseUintList(val)
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

	if config.HostConfig.CpusetCpus == "" {
		return errors.New("CpusetCpus is null,CpusetCpus should not be null")
	}

	_, err := parseCpuset(config)
	if err != nil {
		return err
	}

	return config.Validate()
}

// parse NCPU from config.HostConfig.CpusetCpus
func parseCpuset(config *cluster.ContainerConfig) (int, error) {
	ncpu, err := strconv.Atoi(config.HostConfig.CpusetCpus)
	if err != nil {
		log.WithFields(log.Fields{
			"container ID": config.SwarmID(),
			"CpusetCpus":   config.HostConfig.CpusetCpus,
		}).Errorf("Parse CpusetCpus Error,%s", err)
		return 0, err
	}
	return ncpu, nil
}

// "master:1#standby:1#slave:3"
func getServiceArch(arch string) (map[string]int, int, error) {
	s := strings.Split(arch, "#")
	out, count := make(map[string]int, len(s)), 0

	for i := range s {
		parts := strings.Split(s[i], ":")
		if len(parts) == 1 {
			out[parts[0]] = 1
			count += 1

		} else if len(parts) == 2 {

			if n, err := strconv.Atoi(parts[1]); err == nil {
				out[parts[0]] = n
				count += n

			} else {
				return nil, 0, fmt.Errorf("%s,%s", err, s[i])
			}
		} else {
			return nil, 0, fmt.Errorf("Unexpected format in %s", s[i])
		}
	}

	return out, count, nil
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
