package nic

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/seed/sdk"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func CreateNetworkDevice(ctx context.Context, addr, container string, ips []database.IP, tlsConfig *tls.Config) error {

	for i := range ips {
		config := sdk.NetworkConfig{
			Container:  container,
			HostDevice: ips[i].Bond,
			// ContainerDevice: ips[i].Bond,
			IPCIDR:    fmt.Sprintf("%s/%d", utils.Uint32ToIP(ips[i].IPAddr), ips[i].Prefix),
			Gateway:   ips[i].Gateway,
			VlanID:    ips[i].VLAN,
			BandWidth: ips[i].Bandwidth,
		}

		err := postCreateNetwork(ctx, addr, config, tlsConfig)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func postCreateNetwork(ctx context.Context, addr string, config sdk.NetworkConfig, tlsConfig *tls.Config) error {
	c, err := sdk.NewClient(addr, 30*time.Second, tlsConfig)
	if err != nil {
		return err
	}

	return c.CreateNetwork(ctx, config)
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
