package alloc

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc/nic"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/docker/swarm/seed/sdk"
	"github.com/pkg/errors"
)

type networkAllocOrmer interface {
	ListIPByEngine(ID string) ([]database.IP, error)
	ResetIPs(ips []database.IP) error
	AllocNetworking(unit, engine string, req []database.NetworkingRequire) ([]database.IP, error)
	SetIPs([]database.IP) error
}

type netAllocator struct {
	ec    engineCluster
	ormer networkAllocOrmer
}

func (at netAllocator) AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string,
	networkings []string, requires []structs.NetDeviceRequire) (out []database.IP, err error) {

	idleDevs, width, err := at.availableDevice(engineID)
	if err != nil {
		return nil, err
	}

	for _, req := range requires {
		width = width - req.Bandwidth
	}

	// check network device bandwidth and band
	if width < 0 || len(idleDevs) < len(requires) {
		return nil, errors.Errorf("Engine:%s not enough Bandwidth for require,len(bond)=%d,%d less", engineID, len(idleDevs), width)
	}

	in := make([]database.NetworkingRequire, 0, len(requires))
	for i := range requires {
		in = append(in, database.NetworkingRequire{
			Bond:      idleDevs[i],
			Bandwidth: requires[i].Bandwidth,
		})
	}

	recycle := make([]database.IP, 0, len(in))
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("%v", r)
		}

		if len(recycle) == 0 {
			return
		}
		_err := at.ormer.ResetIPs(recycle)
		if _err != nil {
			logrus.Errorf("alloc networking:%+v", _err)
		}
	}()

	for i := range networkings {
		for l := range in {
			in[l].Networking = networkings[i]
		}
		out, err = at.ormer.AllocNetworking(unitID, engineID, in)
		if err == nil && len(out) == len(in) {
			break
		} else if len(out) > 0 {
			recycle = append(recycle, out...)
		}
	}

	if err != nil {
		return nil, err
	}

	if len(out) != len(in) {
		return nil, errors.New("alloc networkings failed")
	}

	config.HostConfig.NetworkMode = "none"
	for i := range out {
		ip := utils.Uint32ToIP(out[i].IPAddr)
		if ip == nil {
			continue
		}

		config.Config.Env = append(config.Config.Env, "IPADDR="+ip.String())
		break
	}
	return out, nil
}

func (at netAllocator) availableDevice(engineID string) ([]string, int, error) {
	engine := at.ec.Engine(engineID)
	if engine == nil {
		return nil, 0, errors.Errorf("Engine not found:%s", engineID)
	}

	devices, width, err := nic.ParseEngineDevice(engine)
	if err != nil {
		return nil, width, err
	}

	used, err := at.ormer.ListIPByEngine(engine.ID)
	if err != nil {
		return nil, width, err
	}

	list := make([]string, 0, len(devices))
	for d := range devices {
		found := false

		for i := range used {
			if used[i].Bond == devices[d] {
				found = true
				break
			}
		}

		if !found {
			list = append(list, devices[d])
		}
	}

	for i := range used {
		width = width - used[i].Bandwidth
	}

	return list, width, nil
}

func (at netAllocator) AllocDevice(engineID, unitID string, ips []database.IP) ([]database.IP, error) {
	idle, width, err := at.availableDevice(engineID)
	if err != nil {
		return ips, err
	}

	// check network device bandwidth and band

	if len(idle) < len(ips) {
		return ips, errors.Errorf("Engine:%s not enough free net device for require,len(bond)=%d", engineID, len(idle))
	}

	for i := range ips {
		width = width - ips[i].Bandwidth
	}

	if width < 0 {
		return ips, errors.Errorf("Engine:%s not enough Bandwidth for require,%d less", engineID, width)
	}

	out := make([]database.IP, len(ips))
	copy(out, ips)

	for i := range out {
		out[i].Engine = engineID
		out[i].Bond = idle[i]
		out[i].UnitID = unitID
	}

	err = at.ormer.SetIPs(out)
	if err != nil {
		return out, err
	}

	return out, nil
}

func (at netAllocator) UpdateNetworking(ctx context.Context, engineID, addr string, ips []database.IP, width int) error {
	_, free, err := at.availableDevice(engineID)
	if err != nil {
		return err
	}

	for i := range ips {
		free += ips[i].Bandwidth
	}

	if free < len(ips)*width {
		return errors.Errorf("Engine:%s not enough Bandwidth for require,%d less", engineID, free)
	}

	for i := range ips {
		ips[i].Bandwidth = width
	}

	err = at.ormer.SetIPs(ips)
	if err != nil {
		return err
	}

	for i := range ips {
		err := updateNetworkDevice(ctx, addr, ips[i], nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func CreateNetworkDevice(ctx context.Context, addr, container string, ip database.IP, tlsConfig *tls.Config) error {
	config := sdk.NetworkConfig{
		Container:  container,
		HostDevice: ip.Bond,
		// ContainerDevice: ip.Bond,
		IPCIDR:    fmt.Sprintf("%s/%d", utils.Uint32ToIP(ip.IPAddr), ip.Prefix),
		Gateway:   ip.Gateway,
		VlanID:    ip.VLAN,
		BandWidth: ip.Bandwidth,
	}

	err := createNetwork(ctx, addr, config, tlsConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func updateNetworkDevice(ctx context.Context, addr string, ip database.IP, tlsConfig *tls.Config) error {
	config := sdk.NetworkConfig{
		HostDevice: ip.Bond,
		BandWidth:  ip.Bandwidth,
	}

	err := updateNetwork(ctx, addr, config, tlsConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func createNetwork(ctx context.Context, addr string, config sdk.NetworkConfig, tlsConfig *tls.Config) error {
	cli, err := sdk.NewClient(addr, 30*time.Second, tlsConfig)
	if err != nil {
		return err
	}

	return cli.CreateNetwork(ctx, config)
}

func updateNetwork(ctx context.Context, addr string, config sdk.NetworkConfig, tlsConfig *tls.Config) error {
	cli, err := sdk.NewClient(addr, 30*time.Second, tlsConfig)
	if err != nil {
		return err
	}

	return cli.UpdateNetwork(ctx, config)
}
