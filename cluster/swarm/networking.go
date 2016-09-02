package swarm

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/docker/swarm/api/structs"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/cluster/swarm/database"
	"github.com/docker/swarm/utils"
)

var ErrNotFoundIP = errors.New("IP not found")
var ErrNotFoundNetworking = errors.New("Networking not found")

type Networking struct {
	Enable bool
	Prefix int
	*sync.RWMutex
	database.Networking
}

type IP struct {
	allocated bool
	ip        uint32
}

func NewNetworking(net database.Networking, ips []database.IP) (*Networking, error) {
	if len(ips) == 0 {
		return nil, fmt.Errorf("Unsupport Networking With No IP")
	}
	networking := &Networking{
		RWMutex:    new(sync.RWMutex),
		Enable:     true,
		Prefix:     ips[0].Prefix,
		Networking: net,
	}

	return networking, nil
}

func newNetworking(net database.Networking, prefix int) *Networking {
	networking := &Networking{
		RWMutex:    new(sync.RWMutex),
		Enable:     true,
		Prefix:     prefix,
		Networking: net,
	}

	return networking
}

func (gd *Gardener) GetNetworking(id string) (*Networking, error) {
	gd.RLock()

	for i := range gd.networkings {
		if gd.networkings[i].Type == id {
			gd.RUnlock()
			return gd.networkings[i], nil
		}
	}

	gd.RUnlock()

	net, prefix, err := database.GetNetworkingByID(id)
	if err != nil {
		return nil, err
	}

	networking := newNetworking(net, prefix)

	gd.Lock()
	gd.networkings = append(gd.networkings, networking)
	gd.Unlock()

	return networking, nil
}

func (gd *Gardener) GetNetworkingByType(_type string) (*Networking, error) {
	gd.RLock()

	for i := range gd.networkings {
		if gd.networkings[i].Enable &&
			gd.networkings[i].Type == _type {
			gd.RUnlock()
			return gd.networkings[i], nil
		}
	}

	gd.RUnlock()

	out, err := database.ListNetworkingByType(_type)
	if err != nil {
		return nil, err
	}

	list := make([]*Networking, 0, len(out))
	for i := range out {
		ips, err := database.ListIPWithCondition(out[i].ID, false, 1)
		if err != nil || len(ips) == 0 {
			continue
		}

		networking, err := NewNetworking(out[i], ips)
		if err != nil {
			continue
		}

		list = append(list, networking)
	}

	gd.Lock()
	gd.networkings = append(gd.networkings, list...)
	gd.Unlock()

	return list[0], nil
}

func (gd *Gardener) SetNetworkingStatus(ID string, enable bool) error {
	net, err := gd.GetNetworking(ID)
	if err != nil {
		return err
	}

	net.Lock()

	net.Enable = enable
	err = database.UpdateNetworkingStatus(net.ID, enable)

	net.Unlock()

	return err
}

func (gd *Gardener) AddNetworking(start, end, _type, gateway string, prefix int) (*Networking, error) {
	net, ips, err := database.TxInsertNetworking(start, end, gateway, _type, prefix)
	if err != nil {
		return nil, err
	}

	networking, err := NewNetworking(net, ips)
	if err != nil {
		return nil, err
	}

	gd.Lock()

	gd.networkings = append(gd.networkings, networking)

	gd.Unlock()

	return networking, nil
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

func NewIPinfo(net database.Networking, ip database.IP) IPInfo {
	return IPInfo{
		Networking: net.ID,
		IP:         utils.Uint32ToIP(ip.IPAddr),
		Type:       net.Type,
		Gateway:    net.Gateway,
		Prefix:     ip.Prefix,
		ipuint32:   ip.IPAddr,
	}
}

func (info IPInfo) String() string {

	return fmt.Sprintf("%s/%d:%s", info.IP.String(), info.Prefix, info.Device)
}

func getIPInfoByUnitID(ID string, engine *cluster.Engine) ([]IPInfo, error) {
	ips, err := database.ListIPByUnitID(ID)
	if err != nil {
		return nil, err
	}

	out := make([]IPInfo, len(ips))

	for i := range ips {
		net, _, err := database.GetNetworkingByID(ips[i].NetworkingID)
		if err != nil {
			return nil, err
		}

		ip, label := NewIPinfo(net, ips[i]), "bond1"

		if net.Type == _ContainersNetworking {
			ip.Device = "bond1"
			label = _Internal_NIC_Lable
		} else if net.Type == _ExternalAccessNetworking {
			ip.Device = "bond2"
			label = _External_NIC_Lable
		}
		if engine != nil && engine.Labels != nil {
			if dev, ok := engine.Labels[label]; ok {
				ip.Device = dev
			}
		}

		out[i] = ip
	}

	return out, nil
}

func (gd *Gardener) getNetworkingSetting(engine *cluster.Engine, unit string, req []netRequire) ([]IPInfo, error) {
	networkings := make([]IPInfo, 0, 2)
	for _, net := range req {
		networkingID := ""

		if net.Type == _ExternalAccessNetworking {
			dc, err := gd.DatacenterByEngine(engine.ID)
			if err == nil {
				networkingID = strings.TrimSpace(dc.Cluster.NetworkingID)
			}
		}

		ipinfo, err := gd.AllocIP(networkingID, net.Type, unit)
		if err != nil {
			return nil, err
		}

		label := _Internal_NIC_Lable
		if net.Type == _ContainersNetworking {
			label = _Internal_NIC_Lable
			ipinfo.Device = "bond1"
		} else if net.Type == _ExternalAccessNetworking {
			label = _External_NIC_Lable
			ipinfo.Device = "bond2"
		}

		if engine != nil && engine.Labels != nil {
			if device, ok := engine.Labels[label]; ok {
				ipinfo.Device = device
			}
		}

		networkings = append(networkings, ipinfo)
	}

	return networkings, nil
}

func (gd *Gardener) AllocIP(id, _type, unit string) (_ IPInfo, err error) {
	var networkings []database.Networking

	if len(id) > 0 {
		networking, _, err := database.GetNetworkingByID(id)
		if err != nil {
			return IPInfo{}, err
		}
		networkings = []database.Networking{networking}
	}

	if len(networkings) == 0 && len(_type) > 0 {
		networkings, err = database.ListNetworkingByType(_type)
		if err != nil {
			return IPInfo{}, err
		}
	}

	for i := range networkings {
		if !networkings[i].Enabled {
			continue
		}
		ip, err := database.TxAllocIPByNetworking(networkings[i].ID, unit)
		if err == nil {
			return NewIPinfo(networkings[i], ip), nil
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

func (gd *Gardener) RemoveNetworking(ID string) error {
	count, err := database.CountIPByNetwroking(ID, true)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("networking %s is using", ID)
	}

	err = database.TxDeleteNetworking(ID)
	if err != nil {
		return err
	}

	gd.Lock()
	for i := range gd.networkings {
		if gd.networkings[i].ID == ID {
			gd.networkings = append(gd.networkings[:i], gd.networkings[i+1:]...)
			break
		}
	}
	gd.Unlock()

	return nil
}

func ListPorts(start, end, limit int) ([]database.Port, error) {
	if limit == 0 || limit > 1000 {
		limit = 1000
	}

	return database.ListPorts(start, end, limit)
}

func ListNetworkings() ([]structs.ListNetworkingsResponse, error) {
	list, err := database.ListNetworking()
	if err != nil {
		return nil, err
	}

	resp := make([]structs.ListNetworkingsResponse, len(list))
	for i := range list {
		out, err := database.ListIPByNetworking(list[i].ID)
		if err != nil {
			return nil, err
		}
		if len(out) == 0 {
			resp[i] = structs.ListNetworkingsResponse{
				ID:      list[i].ID,
				Type:    list[i].Type,
				Gateway: list[i].Gateway,
				Enabled: list[i].Enabled,
				Mask:    0,
				Total:   0,
				Used:    0,
			}
			continue
		}
		min, max, total, used := statistic(out)

		resp[i] = structs.ListNetworkingsResponse{
			ID:      list[i].ID,
			Type:    list[i].Type,
			Gateway: list[i].Gateway,
			Enabled: list[i].Enabled,
			Mask:    min.Prefix,
			Total:   total,
			Used:    used,
			Start:   utils.Uint32ToIP(min.IPAddr).String(),
			End:     utils.Uint32ToIP(max.IPAddr).String(),
		}
	}

	return resp, nil
}

func statistic(list []database.IP) (database.IP, database.IP, int, int) {
	if len(list) == 0 {
		return database.IP{}, database.IP{}, 0, 0
	}
	min, max, total, used := 0, 0, len(list), 0
	for i := range list {
		if list[i].IPAddr < list[min].IPAddr {
			min = i
		}
		if list[i].IPAddr > list[max].IPAddr {
			max = i
		}
		if list[i].Allocated {
			used++
		}
	}

	return list[min], list[max], total, used
}
