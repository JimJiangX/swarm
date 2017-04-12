package alloc

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/resource/alloc/nic"
	"github.com/docker/swarm/garden/structs"
	"github.com/pkg/errors"
)

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
