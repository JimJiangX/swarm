package alloc

import (
	"testing"

	"github.com/docker/swarm/cluster"
	"github.com/docker/swarm/garden/database"
	"github.com/docker/swarm/garden/structs"
)

type network struct {
	ips []database.IP
}

func (n network) CountIPWithCondition(networking string, allocated bool) (int, error) {
	num := 0

	for _, ip := range n.ips {
		if ip.Networking == networking && ip.UnitID == "" {
			num++
		}
	}

	return num, nil
}

func (n network) ListIPByEngine(engine string) ([]database.IP, error) {
	out := make([]database.IP, 0, 5)

	for _, ip := range n.ips {
		if ip.Engine == engine {
			out = append(out, ip)
		}
	}

	return out, nil
}

func (n *network) SetIPs(ips []database.IP) error {
	for i := range ips {

		for j := range n.ips {
			if n.ips[j].IPAddr == ips[i].IPAddr {
				n.ips[j] = ips[i]
				break
			}
		}
	}

	return nil
}

func (n *network) ResetIPs(ips []database.IP) error {
	for i := range ips {

		for j := range n.ips {
			if n.ips[j].IPAddr == ips[i].IPAddr {
				n.ips[j].Engine = ""
				n.ips[j].Bandwidth = 0
				n.ips[j].Bond = ""
				n.ips[j].UnitID = ""
				break
			}
		}
	}

	return nil
}

func (n *network) AllocNetworking(unit, engine string, req []database.NetworkingRequire) ([]database.IP, error) {

	out := make([]database.IP, 0, len(req))

	for i := range req {
	ips:
		for j := range n.ips {
			if n.ips[j].IPAddr != 0 &&
				n.ips[j].Networking == req[i].Networking &&
				n.ips[j].UnitID == "" && n.ips[j].Enabled {

				n.ips[j].UnitID = unit
				n.ips[j].Engine = engine
				n.ips[j].Bandwidth = req[i].Bandwidth
				n.ips[j].Bond = req[i].Bond

				out = append(out, n.ips[j])
				break ips
			}
		}
	}

	return out, nil
}

func TestAlloctNetworking(t *testing.T) {
	es := engines{
		&cluster.Engine{
			ID:   "engineID0001",
			Name: "engineName0001",
			Labels: map[string]string{
				"PF_DEV_BW":     "1G",
				"CONTAINER_NIC": "bond0,bond1,bond2",
			},
		},
		&cluster.Engine{
			ID:   "engineID0002",
			Name: "engineName0002",
			Labels: map[string]string{
				"PF_DEV_BW":     "600M",
				"CONTAINER_NIC": "bond0,band1",
			},
		},
	}
	ips := &network{
		ips: []database.IP{
			{
				IPAddr:     3232246039,
				Enabled:    true,
				Networking: "networking001",
			},
			{
				IPAddr:     3232246040,
				Enabled:    true,
				Networking: "networking001",
			},
			{
				IPAddr:     3232246041,
				Enabled:    true,
				Networking: "networking001",
			},
			{
				IPAddr:     3232246042,
				Enabled:    true,
				Networking: "networking001",
				UnitID:     "unit000078424",
				Engine:     "engineID0001",
				Bandwidth:  500,
				Bond:       "bond1",
			},
			{
				IPAddr:     3232246043,
				Networking: "networking001",
			},
			{
				IPAddr:     3232246044,
				Enabled:    true,
				Networking: "networking002",
			},
			{
				IPAddr:     3232246045,
				Enabled:    true,
				Networking: "networking002",
				UnitID:     "unit000078424792",
				Engine:     "engineID0002",
				Bandwidth:  400,
				Bond:       "bond0",
			},
			{
				IPAddr:     3232246046,
				Enabled:    true,
				Networking: "networking002",
			},
		}}

	requires := []structs.NetDeviceRequire{
		{
			Bandwidth: 100,
		},
		{
			Bandwidth: 200,
		},
		{
			Bandwidth: 300,
		},
	}

	config := cluster.ContainerConfig{}
	at := netAllocator{
		ec:    es,
		ormer: ips,
	}

	{
		out, err := at.AlloctNetworking(&config, "", "", nil, nil)
		if err != nil {
			t.Errorf("%+v", err)
		} else {
			t.Log(len(out), err)
		}
	}

	{
		out, err := at.AlloctNetworking(&config, "engineID0001", "unit0024794", []string{"networking0011"}, requires[:2])
		if err != nil {
			t.Log("error expected")
		} else {
			t.Error(len(out), err)
		}
	}

	{
		out, err := at.AlloctNetworking(&config, "engineID0001", "unit0024794", []string{"networking001"}, requires[:2])
		if err != nil {
			t.Error(len(out), err)
		} else {
			t.Log(out, err)
		}
	}

	{
		out, err := at.AlloctNetworking(&config, "engineID0001", "unit0024794", []string{"networking001"}, requires[:1])
		if err == nil {
			t.Error("error expected,but got:", len(out))
		} else {
			t.Log(len(out), err)
		}
	}

	{
		out, err := at.AlloctNetworking(&config, "engineID0002", "unit0024794", []string{"networking002"}, requires[:1])
		if err != nil {
			t.Error(len(out), err)
		} else {
			t.Log(out, err)
		}
	}

	{
		out, err := at.AlloctNetworking(&config, "engineID0002", "unit0024794", []string{"networking001", "networking002"}, requires[:1])
		if err == nil {
			t.Error("error expected,but got:", len(out))
		} else {
			t.Log(len(out), err)
		}
	}
}
