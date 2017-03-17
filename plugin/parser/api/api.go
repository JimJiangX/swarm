package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/plugin/client"
	"golang.org/x/net/context"
)

type PluginAPI interface {
	GenerateServiceConfig(ctx context.Context, desc structs.ServiceSpec) (structs.ConfigsMap, error)
	GetUnitConfig(ctx context.Context, service, unit string) (structs.ConfigCmds, error)
	GetCommands(ctx context.Context, service string) (structs.Commands, error)

	GetImageSupport(ctx context.Context) ([]structs.ImageVersion, error)

	PostImageTemplate(ctx context.Context, ct structs.ConfigTemplate) error

	UpdateConfigs(ctx context.Context, service string, configs structs.ConfigsMap) error
	ServiceCompose(ctx context.Context, spec structs.ServiceSpec) error
	ServicesLink(ctx context.Context, links structs.ServicesLink) error
}

type plugin struct {
	c client.Client
}

func NewPlugin(cli client.Client) PluginAPI {
	return plugin{
		c: cli,
	}
}

func (p plugin) GenerateServiceConfig(ctx context.Context, desc structs.ServiceSpec) (structs.ConfigsMap, error) {
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

func (p plugin) GetImageSupport(ctx context.Context) ([]structs.ImageVersion, error) {
	resp, err := client.RequireOK(p.c.Get(ctx, "/image/support"))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var obj []structs.ImageVersion

	err = decodeBody(resp, &obj)

	return obj, err
}

//func (p plugin) GetImageRequirement(ctx context.Context, image string) (structs.RequireResource, error) {
//	params := make(url.Values)
//	params.Set("image", image)

//	url := url.URL{
//		Path:     "/image/requirement",
//		RawQuery: params.Encode(),
//	}

//	var obj structs.RequireResource

//	resp, err := client.RequireOK(p.c.Get(ctx, url.RequestURI()))
//	if err != nil {
//		return obj, err
//	}
//	defer resp.Body.Close()

//	err = decodeBody(resp, &obj)

//	return obj, err
//}

func (p plugin) PostImageTemplate(ctx context.Context, ct structs.ConfigTemplate) error {
	resp, err := client.RequireOK(p.c.Post(ctx, "/image/template", ct))
	if err != nil {
		return err
	}

	client.EnsureBodyClose(resp)

	return nil
}

func (p plugin) UpdateConfigs(ctx context.Context, service string, configs structs.ConfigsMap) error {
	resp, err := client.RequireOK(p.c.Put(ctx, "/configs/"+service, configs))
	if err != nil {
		return err
	}

	client.EnsureBodyClose(resp)

	return nil
}

func (p plugin) ServiceCompose(ctx context.Context, spec structs.ServiceSpec) error {
	uri := fmt.Sprintf("/services/%s/compose", spec.ID)

	resp, err := client.RequireOK(p.c.Put(ctx, uri, spec))
	if err != nil {
		return err
	}

	client.EnsureBodyClose(resp)

	return nil
}

func (p plugin) ServicesLink(ctx context.Context, links structs.ServicesLink) error {
	resp, err := client.RequireOK(p.c.Put(ctx, "/services/link", links))
	if err != nil {
		return err
	}

	client.EnsureBodyClose(resp)

	return nil
}

// decodeBody is used to JSON decode a body
func decodeBody(resp *http.Response, out interface{}) error {
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}
