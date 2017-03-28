package nic

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	defaultTimeout = 30 * time.Second
	createDevURL   = "network/create"
)

type createNetworkConfig struct {
	Container string `json:"containerID"`

	HostDevice      string `json:"hostDevice"`
	ContainerDevice string `json:"containerDevice"`

	IPCIDR  string `json:"IpCIDR"`
	Gateway string `json:"gateway"`

	VlanID    int `json:"vlanID"`
	BandWidth int `json:"bandWidth"`
}

func CreateNetworkDevice(ctx context.Context, addr string, c *cluster.Container, ips []database.IP, tlsConfig *tls.Config) error {
	bond := c.Engine.Labels["adm_nic"]

	for i := range ips {
		config := createNetworkConfig{
			Container:       c.ID,
			HostDevice:      bond,
			ContainerDevice: ips[i].Bond,
			IPCIDR:          fmt.Sprintf("%s/%d", utils.Uint32ToIP(ips[i].IPAddr), ips[i].Prefix),
			Gateway:         ips[i].Gateway,
			VlanID:          ips[i].VLAN,
			BandWidth:       ips[i].Bandwidth,
		}

		err := postCreateNetwork(ctx, addr, config, tlsConfig)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func postCreateNetwork(ctx context.Context, addr string, config createNetworkConfig, tlsConfig *tls.Config) error {
	body := bytes.NewBuffer(nil)
	err := json.NewEncoder(body).Encode(config)
	if err != nil {
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	client, scheme := http.DefaultClient, "http"
	if tlsConfig != nil {
		scheme = "https"
		trans := defaultPooledTransport(defaultTimeout)
		trans.TLSClientConfig = tlsConfig

		client.Transport = trans
	}

	url := fmt.Sprintf("%s://%s/%s", scheme, addr, createDevURL)

	resp, err := ctxhttp.Post(ctx, client, url, "application/json", body)
	if err != nil {
		errors.Wrapf(err, "CreateNetworkDevice %s error", url)
	}
	defer ensureBodyClose(resp)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		_, err := io.Copy(body, resp.Body)

		return errors.Errorf("url=%s,code=%d,out=%s,%v", url, resp.StatusCode, body.String(), err)
	}

	return nil
}

// defaultPooledTransport returns a new http.Transport with similar default
// values to http.DefaultTransport. Do not use this for transient transports as
// it can leak file descriptors over time. Only use this for transports that
// will be re-used for the same host(s).
func defaultPooledTransport(timeout time.Duration) *http.Transport {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: timeout * 2,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
		MaxIdleConnsPerHost: 1,
	}
	return transport
}

// ensureBodyClose close *http.Response
func ensureBodyClose(resp *http.Response) {
	if resp.Body != nil {
		io.CopyN(ioutil.Discard, resp.Body, 512)

		resp.Body.Close()
	}
}
