package swarm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
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
	// TODO: make sure later
	config.AttachStdin = true
	config.AttachStdout = true
	config.AttachStderr = true
	config.Tty = true
	config.OpenStdin = true
	config.StdinOnce = true

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
		config.Labels = make(map[string]string, 5)
	}

	if config.HostConfig.Binds == nil {
		config.HostConfig.Binds = make([]string, 0, 5)
	}

	if config.HostConfig.NetworkMode == "" {
		config.HostConfig.NetworkMode = "host"
	}

	if config.HostConfig.RestartPolicy == (container.RestartPolicy{}) {
		config.HostConfig.RestartPolicy = container.RestartPolicy{
			Name:              "no",
			MaximumRetryCount: 0,
		}
	}

	return config
}

func cloneContainerConfig(config *cluster.ContainerConfig) *cluster.ContainerConfig {
	logrus.Debugf("ContainerConfig %+v", config)

	clone := *config
	clone.ExposedPorts = make(map[nat.Port]struct{})
	clone.Cmd = make([]string, 0, 5)
	clone.Env = make([]string, 0, 5)
	clone.Labels = make(map[string]string, 5)
	clone.HostConfig.Binds = make([]string, 0, 5)
	clone.Volumes = make(map[string]struct{})
	clone.Entrypoint = make([]string, 0)
	clone.OnBuild = make([]string, 0)

	if len(config.Cmd) > 0 {
		clone.Cmd = append(clone.Cmd, config.Cmd...)
	}
	if len(config.Env) > 0 {
		clone.Env = append(clone.Env, config.Env...)
	}
	if len(config.HostConfig.Binds) > 0 {
		clone.HostConfig.Binds = append(clone.HostConfig.Binds, config.HostConfig.Binds...)
	}

	if len(config.ExposedPorts) > 0 {
		for key, value := range config.ExposedPorts {
			clone.ExposedPorts[key] = value
		}
	}

	if len(config.Labels) > 0 {
		for key, value := range config.Labels {
			clone.Labels[key] = value
		}
	}

	return &clone
}

func ParseCPUSets(val string) (map[int]bool, error) {
	return utils.ParseUintList(val)
}

func validateContainerConfig(config *cluster.ContainerConfig) error {
	// validate config
	msg := make([]string, 0, 5)
	swarmID := config.SwarmID()
	if swarmID != "" {
		msg = append(msg, "Swarm ID to the container have created")
	}

	if config.HostConfig.CPUShares != 0 {
		msg = append(msg, "CPUShares > 0,CPUShares should be 0")
	}

	if config.HostConfig.CpusetCpus == "" {
		msg = append(msg, "CpusetCpus is null,CpusetCpus should not be null")
	}

	_, err := parseCpuset(config)
	if err != nil {
		msg = append(msg, err.Error())
	}

	if err := config.Validate(); err != nil {
		msg = append(msg, err.Error())
	}

	if len(msg) == 0 {
		return nil
	}

	return fmt.Errorf("Errors:%s", msg)
}

func validateContainerUpdateConfig(config container.UpdateConfig) error {
	msg := make([]string, 0, 5)
	if config.Resources.CPUShares != 0 {
		msg = append(msg, "CPUShares > 0,CPUShares should be 0")
	}

	if config.Resources.CpusetCpus == "" {
		msg = append(msg, "CpusetCpus is null,CpusetCpus should not be null")
	}

	n, err := strconv.Atoi(config.Resources.CpusetCpus)
	if err != nil {
		msg = append(msg, err.Error())
	} else if n == 0 {
		msg = append(msg, fmt.Sprintf("CpusetCpus is '%s',should >0", config.Resources.CpusetCpus))
	}

	if len(msg) == 0 {
		return nil
	}

	return fmt.Errorf("Errors:%s", msg)
}

// parse NCPU from config.HostConfig.CpusetCpus
func parseCpuset(config *cluster.ContainerConfig) (int, error) {
	ncpu, err := strconv.Atoi(config.HostConfig.CpusetCpus)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"container ID": config.SwarmID(),
			"CpusetCpus":   config.HostConfig.CpusetCpus,
		}).Errorf("Parse CpusetCpus %s Error:%s", config.HostConfig.CpusetCpus, err)

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
