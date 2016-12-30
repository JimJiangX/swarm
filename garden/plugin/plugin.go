package plugin

import (
	"context"

	"github.com/docker/swarm/garden/structs"
)

type plugin struct {
	c *client
}

func (c *client) Plugin() plugin {
	return plugin{c: c}
}

func (p plugin) GenerateServiceConfig(ctx context.Context, spec interface{}) (structs.ConfigsMap, error) {
	var m structs.ConfigsMap

	resp, err := requireOK(p.c.Post(ctx, "/path", spec))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeJSON(resp, &m)

	return m, err
}

func (p plugin) GenerateUnitConfig(ctx context.Context, nameOrID string, args map[string]string) (structs.ConfigCmds, error) {
	var m structs.ConfigCmds

	resp, err := requireOK(p.c.Post(ctx, "/path", args))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeJSON(resp, &m)

	return m, err
}

func (p plugin) GenerateUnitsCmd(ctx context.Context) (structs.Commands, error) {
	var m structs.Commands

	resp, err := requireOK(p.c.Post(ctx, "/path", nil))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeJSON(resp, &m)

	return m, err
}
