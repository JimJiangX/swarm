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
		Prefix:     int(ips[0].Prefix),
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

func (gd *Gardener) GetNetworkingByType(typ string) (*Networking, error) {
	gd.RLock()

	for i := range gd.networkings {
		if gd.networkings[i].Enable &&
			gd.networkings[i].Type == typ {
			gd.RUnlock()
			return gd.networkings[i], nil
		}
	}

	gd.RUnlock()

	out, err := database.ListNetworkingByType(typ)
	if err != nil {
		return nil, err
	}

	list := make([]*Networking, 0, len(out))
	for i := range out {
		ips, err := database.GetMultiIPByNetworking(out[i].ID, false, 1)
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

func (gd *Gardener) AddNetworking(start, end, typ, gateway string, prefix int) (*Networking, error) {
	net, ips, err := database.TxInsertNetworking(start, end, gateway, typ, prefix)
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

func getIPInfoByUnitID(id string, engine *cluster.Engine) ([]IPInfo, error) {
	ips, err := database.ListIPByUnitID(id)
	if err != nil {
		return nil, err
	}

	out := make([]IPInfo, len(ips))

	for i := range ips {
		net, _, err := database.GetNetworkingByID(ips[i].NetworkingID)
		if err != nil {
			return nil, err
		}

		ip := NewIPinfo(net, ips[i])

		if net.Type == _ContainersNetworking {
			device, ok := engine.Labels[_Internal_NIC_Lable]
			if !ok {
				device = "bond1"
			}
			ip.Device = device
		} else if net.Type == _ExternalAccessNetworking {

			device, ok := engine.Labels[_External_NIC_Lable]
			if !ok {
				device = "bond2"
			}
			ip.Device = device
		}

		out[i] = ip
	}

	return out, nil
}

func (gd *Gardener) getNetworkingSetting(engine *cluster.Engine, unit string, req require) ([]IPInfo, error) {
	networkings := make([]IPInfo, 0, 2)
	for _, net := range req.networkings {
		if net.Type == _ContainersNetworking {
			ipinfo, err := gd.AllocIP("", _ContainersNetworking, unit)
			if err != nil {
				return nil, err
			}

			device, ok := engine.Labels[_Internal_NIC_Lable]
			if !ok {
				device = "bond1"
			}
			ipinfo.Device = device

			networkings = append(networkings, ipinfo)

		} else if net.Type == _ExternalAccessNetworking {

			ipinfo, err := gd.AllocIP("", _ExternalAccessNetworking, unit)
			if err != nil {
				return networkings, err
			}

			device, ok := engine.Labels[_External_NIC_Lable]
			if !ok {
				device = "bond2"
			}
			ipinfo.Device = device

			networkings = append(networkings, ipinfo)
		}
	}

	return networkings, nil
}

func (gd *Gardener) AllocIP(id, typ, unit string) (_ IPInfo, err error) {
	var networkings []database.Networking

	if len(id) > 0 {
		networking, _, err := database.GetNetworkingByID(id)
		if err == nil {
			networkings = []database.Networking{networking}
		}
	}

	if len(networkings) == 0 && len(typ) > 0 {
		networkings, err = database.ListNetworkingByType(typ)
		if err != nil {
			return IPInfo{}, err
		}
	}

	for i := range networkings {
		ip, err := database.TXAllocIPByNetworking(networkings[i].ID, unit)
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
	count, err := database.CountIPByNetwrokingAndStatus(ID, true)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("networking %d is using")
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
	if start == 0 && end == 0 {
		return nil, fmt.Errorf("'start' 'end' cannot both be '0'")
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
		min, max, total, used := statistic(out)

		resp[i] = structs.ListNetworkingsResponse{
			ID:      list[i].ID,
			Type:    list[i].Type,
			Gateway: list[i].Gateway,
			Enabled: list[i].Enabled,
			Total:   total,
			Used:    used,
			Start:   utils.Uint32ToIP(min.IPAddr).String(),
			End:     utils.Uint32ToIP(max.IPAddr).String(),
		}
	}

	return resp, nil
}

func statistic(list []database.IP) (database.IP, database.IP, int, int) {
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
