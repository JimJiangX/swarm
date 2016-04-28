package database

import "testing"

func TestSystemConfig(t *testing.T) {
	test := Configurations{
		ConsulConfig: ConsulConfig{
			ConsulIPs:        "146.240.104.23",
			ConsulPort:       8500,
			ConsulDatacenter: "dc1",
			ConsulToken:      "",
			ConsulWaitTime:   15,
		},
		HorusConfig: HorusConfig{
			HorusServerIP:   "10.211.104.23",
			HorusServerPort: 8383,
			HorusEventIP:    "10.211.104.23",
			HorusEventPort:  8484,
		},
		Registry: Registry{},
		SSHDeliver: SSHDeliver{
			SourceDir:   "./script/node-init",
			PkgName:     "",
			ScriptName:  "node-init.sh",
			CA_CRT_Name: "registery-ca.crt",
			Destination: "/tmp",
		},

		DockerPort: 2375,
		PluginPort: 0,
		Retry:      0,
	}
	t.Log(test.DestPath())

	id, err := test.Insert()
	if err != nil {
		t.Fatal(err)
	}

	config, err := GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.ID != int(id) {
		t.Logf("Unexpected:%d != %d", config.ID, id)
	}

	client, err := config.GetConsulClient()
	if err != nil || client == nil {
		t.Fatal(err)
	}

	t.Log(config.GetConsulConfig())
	t.Log(config.GetConsulAddrs())
	t.Log(config.DestPath())
}
