package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

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

func (p plugin) GenerateServiceConfig(ctx context.Context, desc structs.ServiceDesc) (structs.ConfigsMap, error) {
	var m structs.ConfigsMap

	resp, err := client.RequireOK(p.c.Post(ctx, "/configs", desc))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)

	return m, err
}

func (p plugin) GetUnitConfig(ctx context.Context, service, unit string) (structs.ConfigCmds, error) {
	var m structs.ConfigCmds

	uri := fmt.Sprintf("/configs/%s/%s", service, unit)
	resp, err := client.RequireOK(p.c.Get(ctx, uri))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)

	return m, err
}

func (p plugin) GetCommands(ctx context.Context, service string) (structs.Commands, error) {
	var m structs.Commands

	resp, err := client.RequireOK(p.c.Get(ctx, "/commands/"+service))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)

	return m, err
}

func (p plugin) GetImageRequirement(ctx context.Context, name, version string) (structs.RequireResource, error) {
	params := make(url.Values)
	params.Set("name", name)
	params.Set("version", version)

	url := url.URL{
		Path:     "",
		RawQuery: params.Encode(),
	}

	var obj structs.RequireResource

	resp, err := client.RequireOK(p.c.Get(ctx, url.RequestURI()))
	if err != nil {
		return obj, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &obj)

	return obj, err
}

func (p plugin) PostImageTemplate(ctx context.Context, ct structs.ConfigTemplate) error {
	resp, err := client.RequireOK(p.c.Post(ctx, "/image/template", ct))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.CopyN(ioutil.Discard, resp.Body, 512)

	return nil
}

func (p plugin) UpdateConfigs(ctx context.Context, service string, configs structs.ConfigsMap) error {
	resp, err := client.RequireOK(p.c.Post(ctx, "/configs/"+service, configs))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.CopyN(ioutil.Discard, resp.Body, 512)

	return nil
}

// decodeBody is used to JSON decode a body
func decodeBody(resp *http.Response, out interface{}) error {
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}
