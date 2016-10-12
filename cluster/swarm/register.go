package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/pkg/errors"
	"github.com/tatsushid/go-fastping"
)

type registerService struct {
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

func registerToHorus(obj ...registerService) error {
	addr, err := getHorusFromConsul()
	if err != nil {
		return err
	}

	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(obj); err != nil {
		return errors.Wrap(err, "encode registerService")
	}

	url := fmt.Sprintf("http://%s/v1/agent/register", addr)
	resp, err := http.Post(url, "application/json", body)
	if err != nil {
		return errors.Wrap(err, "post agent register to Horus request")
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

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

func deregisterToHorus(force bool, endpoints ...string) error {
	type deregisterService struct {
		Endpoint string
	}

	addr, err := getHorusFromConsul()
	if err != nil {
		return err
	}

	obj := make([]deregisterService, len(endpoints))
	for i := range obj {
		obj[i].Endpoint = endpoints[i]
	}

	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(obj); err != nil {
		return errors.Wrap(err, "encode deregister to Horus")
	}

	path := fmt.Sprintf("http://%s/v1/agent/deregister", addr)

	req, err := http.NewRequest("POST", path, body)
	if err != nil {
		return errors.Wrap(err, "post agent deregister to Horus")
	}
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
	if resp.Body != nil {
		defer resp.Body.Close()
	}

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

// register service to consul and Horus
func registerToServers(u *unit, svc *Service, sys database.Configurations) error {
	if err := registerHealthCheck(u, svc); err != nil {
		logrus.WithField("Unit", u.Name).Errorf("register service health check,%+v", err)
	}

	obj, err := u.registerHorus(sys.MonitorUsername, sys.MonitorPassword, sys.HorusAgentPort)
	if err != nil {
		return err
	}

	err = registerToHorus(obj)
	if err != nil {
		logrus.WithField("Unit", u.Name).Errorf("register To Horus error:%+v", err)
	}

	return err
}

// deregister service to consul and Horus
func deregisterToServices(addr, unitID string) error {
	err := deregisterHealthCheck(addr, unitID)
	if err != nil {
		logrus.WithField("Unit", unitID).Errorf("deregister service to consul,%+v", err)
	}

	err = deregisterToHorus(false, unitID)
	if err != nil {
		logrus.WithField("Endpoints", unitID).Errorf("deregister To Horus:%s", err)

		err = deregisterToHorus(true, unitID)
		if err != nil {
			logrus.WithField("Endpoints", unitID).Errorf("deregister To Horus,force=true,%s", err)
		}
	}

	return err
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
					logrus.Warnf("%s : unreachable %v\n", host, time.Now())
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
