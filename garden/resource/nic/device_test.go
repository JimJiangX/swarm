package nic

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/swarm/cluster"
)

func TestParseDevice(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"bond0,mac_xxxx000,z,192.168.1.0,255.255.255.0,192.168.3.0,65", ""},
		{"bond1,mac_xxxx001,M,192.168.1.1,255.255.255.0,192.168.3.0,98", "bond1,mac_xxxx001,0M,192.168.1.1,255.255.255.0,192.168.3.0,98"},
		{"bond2,mac_xxxx002,,192.168.1.2,255.255.255.0,,199", "bond2,mac_xxxx002,0M,192.168.1.2,255.255.255.0,,199"},
		{"bond3,mac_xxxx003,10m,192.168.1.3,255.255.255.0,192.168.3.0,67", "bond3,mac_xxxx003,10M,192.168.1.3,255.255.255.0,192.168.3.0,67"},
		{"bond4,mac_xxxx004,10G,192.168.1.4,255.255.255.0,192.168.3.0,0", "bond4,mac_xxxx004,10240M,192.168.1.4,255.255.255.0,192.168.3.0,0"},
		{"bond5,mac_xxxx005,10g,192.168.1.5,255.255.255.0,192.168.3.0,99", "bond5,mac_xxxx005,10240M,192.168.1.5,255.255.255.0,192.168.3.0,99"},
		{"mac_xxxx006,10g,192.168.1.6,255.255.255.0,192.168.3.0,vlan_xxxx06", ""},
	}

	for i := range tests {
		d := parseDevice(tests[i].src)
		if tests[i].want == "" && d.err == nil {
			t.Errorf("%s,error expected but got %s", tests[i].src, d)
		}

		if tests[i].want != "" && d.err != nil {
			t.Errorf("%s,error unexpected, but got error:%s", tests[i].src, d.err)
		}

		if tests[i].want != "" && tests[i].want != d.String() {
			t.Errorf("%s,expected %s but got %s", tests[i].src, tests[i].want, d)
		}
	}
}

func TestParseContainerDevice(t *testing.T) {
	tests := []struct {
		key  string
		src  string
		want string
	}{
		{"jaofuaojf", "bond0,mac_xxxx000,z,192.168.1.0,255.255.255.0,192.168.3.0,3", ""},
		{"VF_DEV_17", "bond1,mac_xxxx001,M,192.168.1.1,255.255.255.0,192.168.3.0,4", ""},
		{"VF_DEV_3", "bond2,mac_xxxx002,0M,192.168.1.2,255.255.255.0,192.168.3.0,5", "bond2,mac_xxxx002,0M,192.168.1.2,255.255.255.0,192.168.3.0,5"},
		{"VF_DEV3", "bond3,mac_xxxx003,10m,192.168.1.3,255.255.255.0,192.168.3.0,6", "bond3,mac_xxxx003,10M,192.168.1.3,255.255.255.0,192.168.3.0,6"},
		{"VF_DEV_25", "bond4,mac_xxxx004,10G,192.168.1.4,255.255.255.0,192.168.3.0,7", "bond4,mac_xxxx004,10240M,192.168.1.4,255.255.255.0,192.168.3.0,7"},
		{"VF_DV_3", "bond5,mac_xxxx005,10g,192.168.1.5,255.255.255.0,192.168.3.0,8", "bond5,mac_xxxx005,10240M,192.168.1.5,255.255.255.0,192.168.3.0,8"},
		{"VF_DEV_23", "mac_xxxx006,10g,192.168.1.6,255.255.255.0,192.168.3.0,9", ""},
	}

	c := cluster.Container{
		Container: types.Container{
			Labels: make(map[string]string, 5),
		},
		Config: &cluster.ContainerConfig{
			Config: container.Config{
				Labels: make(map[string]string, 5),
			},
		},
	}

	for i := 0; i < len(tests)/2; i++ {
		c.Container.Labels[tests[i].key] = tests[i].src
	}

	for i := len(tests) / 2; i < len(tests); i++ {
		c.Config.Config.Labels[tests[i].key] = tests[i].src
	}

	devices := parseContainerDevice(&c)
	if len(devices) != 3 {
		t.Errorf("got %d:", len(devices))
	}

	for i := range devices {
		if devices[i].err != nil {
			t.Error(devices[i])
			continue
		}

		t.Log(devices[i])
	}
}
