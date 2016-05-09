package swarm

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

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

func (net *Networking) AllocIP() (uint32, error) {
	list, err := database.GetMultiIPByNetwrokingAndStatus(net.ID, false, 1)
	if err == nil && len(list) == 1 {
		return list[0].IPAddr, nil
	}

	return 0, ErrNotFoundIP
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
		ips, err := database.GetMultiIPByNetwrokingAndStatus(out[i].ID, false, 1)
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

	ipinfo, err := gd.AllocIP("", _ContainersNetworking)
	if err != nil {
		return nil, err
	}

	device, ok := engine.Labels[_Internal_NIC_Lable]
	if !ok {

	}
	ipinfo.Device = device

	networkings = append(networkings, ipinfo)

	if isProxyType(Type) || isProxyType(name) {

		ipinfo2, err := gd.AllocIP("", _ExternalAccessNetworking)
		if err != nil {
			return networkings, err
		}

		device, ok := engine.Labels[_External_NIC_Lable]
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

	gd.RLock()
	for i := range gd.networkings {
		if gd.networkings[i].ID == ID {
			gd.networkings = append(gd.networkings[:i], gd.networkings[i+1:]...)
			break
		}
	}
	gd.RUnlock()

	return nil
}
