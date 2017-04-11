package resource

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/nic"
	"github.com/docker/swarm/garden/structs"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
)

type Networking struct {
	nwo database.NetworkingOrmer
}

func NewNetworks(nwo database.NetworkingOrmer) Networking {
	return Networking{
		nwo: nwo,
	}
}

func (nw Networking) AddNetworking(start, end, gateway, networkingID string, vlan, prefix int) (int, error) {
	startU32 := utils.IPToUint32(start)
	endU32 := utils.IPToUint32(end)

	if move := uint(32 - prefix); (startU32 >> move) != (endU32 >> move) {
		return 0, errors.Errorf("%s-%s is different network segments", start, end)
	}
	if startU32 > endU32 {
		startU32, endU32 = endU32, startU32
	}

	num := int(endU32 - startU32 + 1)
	ips := make([]database.IP, num)
	for i := range ips {
		ips[i] = database.IP{
			IPAddr:     startU32,
			Prefix:     prefix,
			Networking: networkingID,
			VLAN:       vlan,
			Gateway:    gateway,
			Enabled:    true,
		}

		startU32++
	}

	err := nw.nwo.InsertNetworking(ips)
	if err != nil {
		return 0, err
	}

	return len(ips), nil
}

type networkAllocOrmer interface {
	ListIPByEngine(ID string) ([]database.IP, error)
	ResetIPs(ips []database.IP) error
	AllocNetworking(unit, engine string, req []database.NetworkingRequire) ([]database.IP, error)
}

type netAllocator struct {
	ec    engineCluster
	ormer networkAllocOrmer
}

func (at netAllocator) AlloctNetworking(config *cluster.ContainerConfig, engineID, unitID string,
	networkings []string, requires []structs.NetDeviceRequire) (out []database.IP, err error) {

	engine := at.ec.Engine(engineID)
	if engine == nil {
		return nil, errors.New("Engine not found")
	}

	used, err := at.ormer.ListIPByEngine(engine.ID)
	if err != nil {
		return nil, err
	}

	devices, width, err := nic.ParseEngineDevice(engine)
	if err != nil {
		return nil, err
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
	for _, req := range requires {
		width = width - req.Bandwidth
	}

	// check network device bandwidth and band
	if width < 0 || len(list) < len(requires) {
		return nil, errors.Errorf("Engine:%s not enough Bandwidth for require,len(bond)=%d,%d less", engine.Addr, len(list), width)
	}

	in := make([]database.NetworkingRequire, 0, len(requires))
	for i := range requires {
		in = append(in, database.NetworkingRequire{
			Bond:      list[i],
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
		out, err = at.ormer.AllocNetworking(unitID, engine.ID, in)
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

	return out, nil
}
