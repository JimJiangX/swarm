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
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

type result struct {
	val string
	err error
}

func (c *kvClient) registerToHorus(ctx context.Context, obj ...structs.HorusRegistration) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
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

	for i := range obj {
		if obj[i].Node.Select {
			uri := fmt.Sprintf("http://%s/v1/hosts", addr)
			err := postRegister(ctx, uri, obj[i].Node)
			if err != nil {
				return err
			}

		}
		if obj[i].Service.Select {
			uri := fmt.Sprintf("http://%s/v1/units", addr)
			err := postRegister(ctx, uri, obj[i].Service)
			if err != nil {
				return err
			}
		}
	}

	return nil
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

// typ : hosts / containers / units
func (c *kvClient) deregisterToHorus(ctx context.Context, typ, key string, force bool) error {
	var (
		addr string
		ch   = make(chan result, 1)
	)
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

	uri := fmt.Sprintf("http://%s/v1/%s/%s", addr, typ, key)

	req, err := http.NewRequest(http.MethodDelete, uri, nil)
	if err != nil {
		return errors.Wrap(err, "deregister to Horus")
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

// RegisterService register service to consul and Horus
func (c *kvClient) RegisterService(ctx context.Context, host string, config structs.ServiceRegistration) error {
	if config.Consul != nil {
		err := c.registerHealthCheck(host, *config.Consul)
		if err != nil {
			return err
		}
	}

	if config.Horus != nil {
		select {
		default:
			return c.registerToHorus(ctx, *config.Horus)

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// DeregisterService service to consul and Horus
func (c *kvClient) DeregisterService(ctx context.Context, typ, key string) error {
	err := c.deregisterToHorus(ctx, typ, key, false)
	if err != nil {
		err = c.deregisterToHorus(ctx, typ, key, true)
	}

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

func (c *kvClient) GetHorusAddr() (string, error) {
	addr, client, err := c.getClient("")
	if err != nil {
		return "", err
	}

	checks, _, err := client.Health().State("passing", nil)
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
