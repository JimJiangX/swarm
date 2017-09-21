package database

import (
	"testing"

	"github.com/docker/swarm/garden/utils"
)

func TestCombin(t *testing.T) {
	tests := []NetworkingRequire{
		{Networking: "aaaaa", Bandwidth: 1},
	}

	add := []NetworkingRequire{
		{Networking: "bbbbb", Bandwidth: 2},
		{Networking: "ccccc", Bandwidth: 3},
		{Networking: "bbbbb", Bandwidth: 4},
		{Networking: "aaaaa", Bandwidth: 5},
		{Networking: "aaaaa", Bandwidth: 6},
	}

	out := combin(nil)
	if out != nil {
		t.Errorf("Unexpected,len(out)=%d", len(out))
	}

	out = combin(tests)
	if len(out) != 1 {
		t.Errorf("Unexpected,len(out)=%d", len(out))
	}

	t.Log(out)

	tests = append(tests, add[0:2]...)
	out = combin(tests)
	if len(out) != 3 {
		t.Errorf("Unexpected,len(tests)=%d len(out)=%d", len(tests), len(out))
	}

	t.Log(out)

	tests = append(tests, add[2:]...)
	out = combin(tests)
	if len(out) != 3 {
		t.Errorf("Unexpected,len(tests)=%d len(out)=%d", len(tests), len(out))
	}

	t.Log(out)
}

func TestNetworking(t *testing.T) {
	if ormer == nil {
		t.Skip("orm:db is required")
	}

	networks := []string{utils.Generate64UUID(), utils.Generate64UUID()}
	ips := make([]IP, 50)
	gateway := utils.Generate32UUID()

	for i := range ips {
		ips[i].IPAddr = 4232237000 + uint32(i)
		ips[i].Prefix = 24
		ips[i].Networking = networks[i&0x01]
		ips[i].Gateway = gateway
		ips[i].Enabled = i&0x03 != 3
		ips[i].VLAN = i&0x01 + 1
	}

	err := ormer.InsertNetworking(ips)
	if err != nil {
		t.Errorf("%+v", err)
	}

	for i := range networks {
		defer func(id string) {
			err := ormer.DelNetworking(id)
			if err != nil {
				t.Errorf("%+v", err)
			}
		}(networks[i])
	}

	req := []NetworkingRequire{
		{
			Networking: networks[0],
			Bond:       utils.Generate16UUID(),
			Bandwidth:  1000,
		},
		{
			Networking: networks[0],
			Bond:       utils.Generate16UUID(),
			Bandwidth:  1000,
		},
		{
			Networking: networks[0],
			Bond:       utils.Generate16UUID(),
			Bandwidth:  1000,
		},
		{
			Networking: networks[0],
			Bond:       utils.Generate16UUID(),
			Bandwidth:  1000,
		},
		{
			Networking: networks[1],
			Bond:       utils.Generate16UUID(),
			Bandwidth:  1000,
		},
		{
			Networking: networks[1],
			Bond:       utils.Generate16UUID(),
			Bandwidth:  1000,
		},
		{
			Networking: networks[1],
			Bond:       utils.Generate16UUID(),
			Bandwidth:  1000,
		},
	}

	out, err := ormer.AllocNetworking(utils.Generate64UUID(), utils.Generate128UUID(), req)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if len(out) != len(req) {
		t.Errorf("expected %d but got %d", len(out), len(req))
	}

	err = ormer.DelNetworking(networks[0])
	if err == nil {
		t.Error("expected del networking failed,networking in used")
	}

	err = ormer.ResetIPs(out)
	if err != nil {
		t.Errorf("%+v", err)
	}
}
