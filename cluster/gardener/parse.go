package gardener

import "github.com/samalba/dockerclient"

func defaultContainerConfig() *dockerclient.ContainerConfig {
	return &dockerclient.ContainerConfig{
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
	}
}

func buildContainerConfig(config *dockerclient.ContainerConfig) *dockerclient.ContainerConfig {

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

	if config.Volumes == nil {
		config.Volumes = make(map[string]struct{})
	}

	if config.Volumes == nil {
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
