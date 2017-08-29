package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type pclient interface {
	Get(ctx context.Context, url string) (*http.Response, error)

	Post(ctx context.Context, url string, body interface{}) (*http.Response, error)

	Put(ctx context.Context, url string, body interface{}) (*http.Response, error)
}

// PluginAPI is a plugin server HTTP API client.
type PluginAPI interface {
	GenerateServiceConfig(ctx context.Context, desc structs.ServiceSpec) (structs.ConfigsMap, error)
	GenerateUnitConfig(ctx context.Context, unit string, desc structs.ServiceSpec) (structs.ConfigCmds, error)

	GetUnitConfig(ctx context.Context, service, unit string) (structs.ConfigCmds, error)
	GetServiceConfig(ctx context.Context, service string) (structs.ConfigsMap, error)
	GetCommands(ctx context.Context, service string) (structs.Commands, error)

	GetImage(ctx context.Context, version string) (structs.ConfigTemplate, error)
	GetImageSupport(ctx context.Context) ([]structs.ImageVersion, error)

	PostImageTemplate(ctx context.Context, ct structs.ConfigTemplate) error

	UpdateConfigs(ctx context.Context, service string, configs structs.ConfigsMap) error
	ServiceCompose(ctx context.Context, spec structs.ServiceSpec) error
	ServicesLink(ctx context.Context, links structs.ServicesLink) error
}

type plugin struct {
	host string
	c    pclient
}

// NewPlugin returns a PluginAPI
func NewPlugin(host string, cli pclient) PluginAPI {
	return plugin{
		host: host,
		c:    cli,
	}
}

func (p plugin) GenerateServiceConfig(ctx context.Context, desc structs.ServiceSpec) (structs.ConfigsMap, error) {
	var m structs.ConfigsMap
	const uri = "/configs"

	resp, err := requireOK(p.c.Post(ctx, uri, desc))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)
	if err != nil {
		return nil, errors.Errorf("%s %s%s,%v", http.MethodPost, p.host, uri, err)
	}

	return m, err
}

func (p plugin) GenerateUnitConfig(ctx context.Context, unit string, desc structs.ServiceSpec) (structs.ConfigCmds, error) {
	var (
		cc  structs.ConfigCmds
		uri = "/configs/" + unit
	)

	resp, err := requireOK(p.c.Post(ctx, uri, desc))
	if err != nil {
		return cc, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &cc)
	if err != nil {
		return cc, errors.Errorf("%s %s%s,%v", http.MethodPost, p.host, uri, err)
	}

	return cc, err
}

func (p plugin) GetServiceConfig(ctx context.Context, service string) (structs.ConfigsMap, error) {
	var (
		m   structs.ConfigsMap
		uri = "/configs/" + service
	)

	resp, err := requireOK(p.c.Get(ctx, uri))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)
	if err != nil {
		return nil, errors.Errorf("%s %s%s,%v", http.MethodGet, p.host, uri, err)
	}

	return m, err
}

func (p plugin) GetUnitConfig(ctx context.Context, service, unit string) (structs.ConfigCmds, error) {
	var (
		m   structs.ConfigCmds
		uri = fmt.Sprintf("/configs/%s/%s", service, unit)
	)

	resp, err := requireOK(p.c.Get(ctx, uri))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)
	if err != nil {
		return m, errors.Errorf("%s %s%s,%v", http.MethodGet, p.host, uri, err)
	}

	return m, err
}

func (p plugin) GetCommands(ctx context.Context, service string) (structs.Commands, error) {
	var (
		m   structs.Commands
		uri = "/commands/" + service
	)
	resp, err := requireOK(p.c.Get(ctx, uri))
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &m)
	if err != nil {
		return nil, errors.Errorf("%s %s%s,%v", http.MethodGet, p.host, uri, err)
	}

	return m, err
}

func (p plugin) GetImage(ctx context.Context, version string) (structs.ConfigTemplate, error) {
	obj := structs.ConfigTemplate{}
	uri := "/image/template/" + version

	resp, err := requireOK(p.c.Get(ctx, uri))
	if err != nil {
		return obj, err
	}
	defer resp.Body.Close()

	err = decodeBody(resp, &obj)
	if err != nil {
		return obj, errors.Errorf("%s %s%s,%v", http.MethodGet, p.host, uri, err)
	}

	return obj, err
}

func (p plugin) GetImageSupport(ctx context.Context) ([]structs.ImageVersion, error) {
	const uri = "/image/support"

	resp, err := requireOK(p.c.Get(ctx, uri))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var obj []structs.ImageVersion

	err = decodeBody(resp, &obj)
	if err != nil {
		return nil, errors.Errorf("%s %s%s,%v", http.MethodGet, p.host, uri, err)
	}

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
//  if err != nil {
//		return obj, errors.Errorf("%s %s%s,%v", http.MethodGet, p.host, url.String(), err)
//	}

//	return obj, err
//}

func (p plugin) PostImageTemplate(ctx context.Context, ct structs.ConfigTemplate) error {
	resp, err := requireOK(p.c.Post(ctx, "/image/template", ct))
	if err != nil {
		return err
	}

	ensureBodyClose(resp)

	return nil
}

func (p plugin) UpdateConfigs(ctx context.Context, service string, configs structs.ConfigsMap) error {
	resp, err := requireOK(p.c.Put(ctx, "/configs/"+service, configs))
	if err != nil {
		return err
	}

	ensureBodyClose(resp)

	return nil
}

func (p plugin) ServiceCompose(ctx context.Context, spec structs.ServiceSpec) error {
	uri := fmt.Sprintf("/services/%s/compose", spec.ID)

	resp, err := requireOK(p.c.Put(ctx, uri, spec))
	if err != nil {
		return err
	}

	ensureBodyClose(resp)

	return nil
}

func (p plugin) ServicesLink(ctx context.Context, links structs.ServicesLink) error {
	resp, err := requireOK(p.c.Post(ctx, "/services/link", links))
	if err != nil {
		return err
	}

	ensureBodyClose(resp)

	return nil
}

// decodeBody is used to JSON decode a body
func decodeBody(resp *http.Response, out interface{}) error {
	dec := json.NewDecoder(resp.Body)

	return dec.Decode(out)
}

// requireOK is used to wrap doRequest and check for a 200
func requireOK(resp *http.Response, e error) (*http.Response, error) {
	if e != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, e
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		buf := bytes.NewBuffer(nil)

		io.Copy(buf, resp.Body)
		resp.Body.Close()

		return nil, errors.Errorf("%s,Unexpected response code: %d (%s)", resp.Request.URL.String(), resp.StatusCode, buf.Bytes())
	}

	return resp, nil
}

// ensureBodyClose close *http.Response
func ensureBodyClose(resp *http.Response) {
	if resp.Body != nil {
		io.CopyN(ioutil.Discard, resp.Body, 512)

		resp.Body.Close()
	}
}
