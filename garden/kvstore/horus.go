package kvstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/docker/swarm/garden/structs"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

type result struct {
	val string
	err error
}

func (c *kvClient) registerToHorus(ctx context.Context, obj structs.HorusRegistration) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}

	var (
		addr string
		ch   = make(chan result, 1)
	)
	go func(ch chan<- result) {
		addr, err := c.GetHorusAddr(ctx)
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

	if obj.Node.Select {
		uri := fmt.Sprintf("http://%s/v1/%s", addr, hostType)
		err := postRegister(ctx, uri, obj.Node)
		if err != nil {
			return err
		}
	}

	if obj.Service.Select {
		uri := fmt.Sprintf("http://%s/v1/%s", addr, unitType)
		err := postRegister(ctx, uri, obj.Service)
		if err != nil {
			return err
		}
	}

	return nil
}

type errorResponse struct {
	Result  bool        `json:"result"`
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Object  interface{} `json:"object"`
}

func (res errorResponse) String() string {
	return fmt.Sprintf("%d:%s", res.Code, res.Message)
}

func postRegister(ctx context.Context, uri string, obj interface{}) error {
	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(obj); err != nil {
		return errors.Wrap(err, "encode registerService")
	}

	resp, err := ctxhttp.Post(ctx, nil, uri, "application/json", body)
	if err != nil {
		return errors.Wrap(err, "register to Horus response")
	}
	defer ensureReaderClosed(resp)

	if resp.StatusCode != http.StatusCreated {
		err := readResponseError(resp)
		if err != nil {
			return errors.Wrapf(err, "%s code=%d,error=%s", uri, resp.StatusCode, err)
		}
	}

	return nil
}

// typ : hosts / containers / units
func (c *kvClient) deregisterToHorus(ctx context.Context, config structs.ServiceDeregistration, force bool) error {
	var (
		addr string
		ch   = make(chan result, 1)
	)
	go func(ch chan<- result) {
		addr, err := c.GetHorusAddr(ctx)
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

	uri := fmt.Sprintf("http://%s/v1/%s/%s", addr, config.Type, config.Key)

	del := false
	if config.Type == unitType || config.Type == containerType {
		del = true
	}

	req, err := http.NewRequest(http.MethodDelete, uri, nil)
	if err != nil {
		return errors.Wrap(err, "deregister to Horus")
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	if force || del || config.User != "" {
		params := make(url.Values)
		if force {
			params.Set("force", "true")
		}
		if del {
			params.Set("del_container", "true")
		}
		if config.User != "" {
			params.Set("os_user", config.User)
			params.Set("os_pwd", config.Password)
		}
		req.URL.RawQuery = params.Encode()
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "deregister to Horus response")
	}
	defer ensureReaderClosed(resp)

	if resp.StatusCode != http.StatusNoContent {
		err := readResponseError(resp)
		if err != nil {
			return errors.Wrapf(err, "%s code=%d,error=%s", uri, resp.StatusCode, err)
		}
	}

	return nil
}

// RegisterService register service to consul and Horus
func (c *kvClient) RegisterService(ctx context.Context, host string, config structs.ServiceRegistration) error {
	if config.Consul != nil {
		err := c.registerHealthCheck(host, *config.Consul)
		if err != nil {
			return err
		}
	}

	if config.Horus != nil {
		return c.registerToHorus(ctx, *config.Horus)
	}

	return nil
}

// DeregisterService service to consul and Horus
func (c *kvClient) DeregisterService(ctx context.Context, config structs.ServiceDeregistration, force bool) error {
	err := c.deregisterToHorus(ctx, config, force)

	return err

	//	if err != nil {
	//		return err
	//	}

	//	select {
	//	default:
	//		err = c.deregisterHealthCheck(addr, key)
	//	case <-ctx.Done():
	//		return ctx.Err()
	//	}

	//	return err
}

func (c *kvClient) GetHorusAddr(ctx context.Context) (string, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return "", err
	}

	var q *api.QueryOptions
	if ctx != nil {
		q = &api.QueryOptions{
			Context: ctx,
		}
	}

	checks, _, err := client.Health().State(api.HealthPassing, q)
	c.checkConnectError(addr, err)
	if err != nil {
		return "", errors.Wrap(err, "passing health checks")
	}

	for i := range checks {
		addr := parseIPFromHealthCheck(checks[i].ServiceName, checks[i].Output)
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
	addr := serviceID[index+len(key):]

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

/*
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
*/

func ensureReaderClosed(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		// Drain up to 512 bytes and close the body to let the Transport reuse the connection
		io.CopyN(ioutil.Discard, resp.Body, 512)
		resp.Body.Close()
	}
}

type responseErrorHead struct {
	Result   bool        `json:"result"`
	Code     int         `json:"code"`
	Msg      string      `json:"msg"`
	Category string      `json:"category"`
	Object   interface{} `json:"object"`
}

func (r responseErrorHead) Error() string {
	return fmt.Sprintf("%d:%s:%s", r.Code, r.Category, r.Msg)
}

func readResponseError(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}

	if resp.Header.Get("Content-Type") == "application/json" {
		h := responseErrorHead{}
		err := json.NewDecoder(resp.Body).Decode(&h)
		if err != nil {
			return err
		}

		return h
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return fmt.Errorf("Body:%s", body)
}
