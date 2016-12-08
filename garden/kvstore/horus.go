package kvstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/tatsushid/go-fastping"
)

type RegisterHorusService struct {
	Endpoint      string
	CollectorName string   `json:"collectorname,omitempty"`
	User          string   `json:"user,omitempty"`
	Password      string   `json:"pwd,omitempty"`
	Type          string   `json:"type"`
	CollectorIP   string   `json:"colletorip"`   // spell error
	CollectorPort int      `json:"colletorport"` // spell error
	MetricTags    string   `json:"metrictags"`
	Network       []string `json:"network,omitempty"`
	Status        string   `json:"status"`
	Table         string   `json:"table"`
	CheckType     string   `json:"checktype"`
}

type result struct {
	val string
	err error
}

func (c *kvClient) registerToHorus(ctx context.Context, obj ...RegisterHorusService) error {
	var addr string
	ch := make(chan result, 1)
	go func(ch chan<- result) {
		addr, err := c.GetHorusAddr()
		ch <- result{
			val: addr,
			err: err,
		}
		close(ch)
	}(ch)

	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		addr = r.val

	case <-ctx.Done():
		return ctx.Err()
	}

	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(obj); err != nil {
		return errors.Wrap(err, "encode registerService")
	}

	uri := fmt.Sprintf("http://%s/v1/agent/register", addr)
	req, err := http.NewRequest("POST", uri, body)
	if err != nil {
		return errors.Wrap(err, "post agent register to Horus")
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "register to Horus response")
	}
	defer ensureReaderClosed(resp)

	if resp.StatusCode != http.StatusOK {
		res := struct {
			Err string
		}{}

		err := json.NewDecoder(resp.Body).Decode(&res)
		if err != nil {
			return errors.Wrap(err, "decode response body")
		}
		return errors.Errorf("StatusCode:%d,Error:%s", resp.StatusCode, res.Err)
	}

	return nil
}

func (c *kvClient) deregisterToHorus(ctx context.Context, force bool, endpoints ...string) error {
	type deregisterService struct {
		Endpoint string
	}

	var addr string
	ch := make(chan result, 1)

	go func(ch chan<- result) {
		addr, err := c.GetHorusAddr()
		ch <- result{
			val: addr,
			err: err,
		}
		close(ch)
	}(ch)

	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		addr = r.val

	case <-ctx.Done():
		return ctx.Err()
	}

	obj := make([]deregisterService, len(endpoints))
	for i := range obj {
		obj[i].Endpoint = endpoints[i]
	}

	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(obj); err != nil {
		return errors.Wrap(err, "encode deregister to Horus")
	}

	uri := fmt.Sprintf("http://%s/v1/agent/deregister", addr)

	req, err := http.NewRequest("POST", uri, body)
	if err != nil {
		return errors.Wrap(err, "post agent deregister to Horus")
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	if force {
		params := make(url.Values)
		params.Set("force", "true")
		req.URL.RawQuery = params.Encode()
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "deregister to Horus response")
	}
	defer ensureReaderClosed(resp)

	if resp.StatusCode != http.StatusOK {
		res := struct {
			Err string
		}{}

		err := json.NewDecoder(resp.Body).Decode(&res)
		if err != nil {
			return errors.Wrap(err, "decode Horus response body")
		}
		return errors.Errorf("StatusCode:%d,Error:%s", resp.StatusCode, res.Err)
	}

	return nil
}

// registerToServers register service to consul and Horus
func (c *kvClient) registerToServers(ctx context.Context, host string, config api.AgentServiceRegistration, obj RegisterHorusService) []error {
	errs := make([]error, 0, 2)

	err := c.registerHealthCheck(host, config)
	if err != nil {
		errs = append(errs, err)
	}

	select {
	default:
	case <-ctx.Done():
		errs = append(errs, ctx.Err())
		return errs
	}

	err = c.registerToHorus(ctx, obj)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}

	return errs
}

// deregister service to consul and Horus
func (c *kvClient) deregisterToServices(ctx context.Context, addr, ID string) error {
	err := c.deregisterToHorus(ctx, false, ID)
	if err != nil {

		err = c.deregisterToHorus(ctx, true, ID)
	}

	if err != nil {
		return err
	}

	select {
	default:
		err = c.deregisterHealthCheck(addr, ID)
	case <-ctx.Done():
		return ctx.Err()
	}

	return err
}

func (c *kvClient) GetHorusAddr() (string, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return "", err
	}

	checks, _, err := client.healthChecks("passing", nil)
	c.checkConnectError(addr, err)
	if err != nil {
		return "", errors.Wrap(err, "passing health checks")
	}

	for i := range checks {
		addr := parseIPFromHealthCheck(checks[i].ServiceID, checks[i].Output)
		if addr != "" {
			return addr, nil
		}
	}

	return "", errors.New("non-available Horus query from KV store")
}

func parseIPFromHealthCheck(serviceID, output string) string {
	const key = "HS-"

	if !strings.HasPrefix(serviceID, key) {
		return ""
	}

	index := strings.Index(serviceID, key)
	addr := string(serviceID[index+len(key):])

	if net.ParseIP(addr) == nil {
		return ""
	}

	index = strings.Index(output, addr)

	parts := strings.Split(string(output[index:]), ":")
	if len(parts) >= 2 {

		addr = parts[0] + ":" + parts[1]
		_, _, err := net.SplitHostPort(addr)
		if err == nil {
			return addr
		}
	}

	return ""
}

// fastPing sends an ICMP packet and wait a response,
// when udp is true,use non-privileged datagram-oriented UDP as ICMP endpoints
func fastPing(hostname string, count int, udp bool) (bool, error) {
	type response struct {
		addr *net.IPAddr
		rtt  time.Duration
	}

	p := fastping.NewPinger()
	if udp {
		p.Network("udp")
	}

	netProto := "ip4:icmp"
	if strings.Index(hostname, ":") != -1 {
		netProto = "ip6:ipv6-icmp"
	}
	ra, err := net.ResolveIPAddr(netProto, hostname)
	if err != nil {
		return false, err
	}

	results := make(map[string]*response)
	results[ra.String()] = nil
	p.AddIPAddr(ra)

	onRecv, onIdle := make(chan *response), make(chan bool)
	p.OnRecv = func(addr *net.IPAddr, t time.Duration) {
		onRecv <- &response{addr: addr, rtt: t}
	}
	p.OnIdle = func() {
		onIdle <- true
	}

	p.MaxRTT = time.Millisecond * 200
	p.RunLoop()
	defer p.Stop()

	for i, reach := 0, 0; i < count; i++ {
		select {
		case res := <-onRecv:
			if _, ok := results[res.addr.String()]; ok {
				results[res.addr.String()] = res
			}

			if res.addr.String() == hostname {
				reach++
			}
			if reach > count/2 {
				return true, nil
			}
		case <-onIdle:
			for host, r := range results {
				if r == nil {
					fmt.Printf("%s : unreachable %v\n", host, time.Now())
				}
				results[host] = nil
			}
		case <-p.Done():
			if err = p.Err(); err != nil {
				return false, err
			}
		}
	}

	return false, errors.New(hostname + ":unreachable")
}

func ensureReaderClosed(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		// Drain up to 512 bytes and close the body to let the Transport reuse the connection
		io.CopyN(ioutil.Discard, resp.Body, 512)
		resp.Body.Close()
	}
}
