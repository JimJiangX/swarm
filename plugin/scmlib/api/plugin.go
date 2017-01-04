package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/plugin/client"
)

type plugin struct {
	c client.Client
}

func NewPlugin(cli client.Client) plugin {
	return plugin{
		c: cli,
	}
}

func (p plugin) GenerateServiceConfig(ctx context.Context, spec interface{}) (structs.ConfigsMap, error) {
	var m structs.ConfigsMap

	resp, err := p.c.RequireOK(p.c.Post(ctx, "/path", spec))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)

	return m, err
}

func (p plugin) GenerateUnitConfig(ctx context.Context, nameOrID string, args map[string]string) (structs.ConfigCmds, error) {
	var m structs.ConfigCmds

	resp, err := p.c.RequireOK(p.c.Post(ctx, "/path", args))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)

	return m, err
}

func (p plugin) GenerateUnitsCmd(ctx context.Context) (structs.Commands, error) {
	var m structs.Commands

	resp, err := p.c.RequireOK(p.c.Post(ctx, "/path", nil))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)

	return m, err
}

// decodeBody is used to JSON decode a body
func decodeBody(resp *http.Response, out interface{}) error {
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}
