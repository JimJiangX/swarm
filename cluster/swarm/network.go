package swarm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/utils"
)

const (
	NodesNetworking          = "nodes networking"
	ContainersNetworking     = "containers networking"
	ExternalAccessNetworking = "External Access Networking"

	networkingLabelKey      = "upm.ip"
	proxynetworkingLabelKey = "upm.proxyip"
)

var ErrNotFoundIP = errors.New("IP not found")

type Networking struct {
	Enable  bool
	Rlock   *sync.RWMutex
	ID      string
	IP      string
	Type    string
	Gateway string
	Prefix  int
	pool    []*IP
}

type IP struct {
	allocated bool
	ip        uint32
}

func NewNetworking(id, ip, typ, gateway string, prefix, num int) *Networking {
	net := &Networking{
		Rlock:   new(sync.RWMutex),
		ID:      id,
		Type:    typ,
		IP:      ip,
		Prefix:  prefix,
		Gateway: gateway,
		pool:    make([]*IP, num),
	}

	addrU32 := utils.IPToUint32(net.ID)

	for i := 0; i < num; i++ {
		net.pool[i] = &IP{
			ip:        addrU32,
			allocated: false,
		}

		addrU32++
	}

	return net
}

func (net *Networking) AllocIP() (uint32, error) {
	net.Rlock.Lock()
	defer net.Rlock.Unlock()

	for i := range net.pool {
		if !net.pool[i].allocated {
			net.pool[i].allocated = true

			return net.pool[i].ip, nil
		}
	}

	return 0, ErrNotFoundIP
}

func (gd *Gardener) GetNetworking(typ string) *Networking {
	gd.RLock()

	for i := range gd.networkings {
		if gd.networkings[i].Enable &&
			gd.networkings[i].Type == typ {
			gd.RUnlock()
			return gd.networkings[i]
		}
	}

	gd.RUnlock()

	return nil
}

type IPInfo struct {
	Device     string
	IP         net.IP
	Networking string
	Type       string
	Gateway    string
	Prefix     int
	ipuint32   uint32
}

func NewIPinfo(net *Networking, ip uint32) IPInfo {
	return IPInfo{
		Networking: net.ID,
		IP:         utils.Uint32ToIP(ip),
		Type:       net.Type,
		Gateway:    net.Gateway,
		Prefix:     net.Prefix,
		ipuint32:   ip,
	}
}

func (info IPInfo) String() string {

	return fmt.Sprintf("%s/%d:%s", info.IP.String(), info.Prefix, info.Device)
}

func (gd *Gardener) getNetworkingSetting(engine *cluster.Engine, name, Type string) ([]IPInfo, error) {
	networkings := make([]IPInfo, 0, 2)

	ipinfo, err := gd.AllocIP("", ContainersNetworking)
	if err != nil {
		return nil, err
	}

	device, ok := engine.Labels["internal_NIC"]
	if !ok {

	}
	ipinfo.Device = device

	networkings = append(networkings, ipinfo)

	if isProxyType(Type) || isProxyType(name) {

		ipinfo2, err := gd.AllocIP("", ExternalAccessNetworking)
		if err != nil {
			return networkings, err
		}

		device, ok := engine.Labels["external_NIC"]
		if !ok {

		}
		ipinfo2.Device = device

		networkings = append(networkings, ipinfo2)
	}

	return networkings, err
}

func (gd *Gardener) AllocIP(id, typ string) (IPInfo, error) {
	for i := range gd.networkings {
		if !gd.networkings[i].Enable {
			continue
		}

		if (id != "" && id == gd.networkings[i].ID) ||
			(typ != "" && typ == gd.networkings[i].Type) {

			ip, err := gd.networkings[i].AllocIP()
			if err != nil {
				continue
			}

			return NewIPinfo(gd.networkings[i], ip), nil
		}
	}

	return IPInfo{}, ErrNotFoundIP
}

func isProxyType(name string) bool {

	if strings.Contains(name, "proxy") {
		return true
	}

	return false
}

type createNetworkingRequest struct {
	Device string `json:"Device"`
	IPCIDR string `json:"IpCIDR"` // ip/netmask
}

func createNetworkingAPI(addr, ip, device string, netmask int) error {
	obj := createNetworkingRequest{
		Device: device,
		IPCIDR: fmt.Sprintf("%s/%d", ip, netmask),
	}

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(obj); err != nil {
		return err
	}

	resp, err := http.Post(addr, "application/json", buf)
	if err != nil {
		return err
	}

	if resp.Body != nil {
		b, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return &statusError{resp.StatusCode, err.Error()}
		}

		remoteErr := responseErr{}
		if err := json.Unmarshal(b, &remoteErr); err == nil {
			if remoteErr.Err != "" ||
				resp.StatusCode < http.StatusOK ||
				resp.StatusCode >= http.StatusBadRequest {
				return &statusError{resp.StatusCode, remoteErr.Err}
			}
		}

		// old way...
		return &statusError{resp.StatusCode, string(b)}
	}

	return nil
}

// Response(s) should have an Err field indicating what went
// wrong. Try to unmarshal into ResponseErr. Otherwise fallback to just
// return the string(body)
type responseErr struct {
	Err string
}

type statusError struct {
	status int
	err    string
}

// Error returns a formatted string for this error type
func (e *statusError) Error() string {
	return fmt.Sprintf("StatusCode:%d %v", e.status, e.err)
}
