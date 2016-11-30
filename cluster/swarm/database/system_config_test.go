package database

import "testing"

func deleteSystemConfig(id int64) error {
	db, err := getDB(false)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM tbl_dbaas_system_config WHERE dc_id=?", id)

	return err
}

func TestSystemConfig(t *testing.T) {
	test := Configurations{
		ConsulConfig: ConsulConfig{
			ConsulIPs:        "146.240.104.23,146.240.104.24,146.240.104.25",
			ConsulPort:       8500,
			ConsulDatacenter: "dc1",
			ConsulToken:      "",
			ConsulWaitTime:   15,
		},
		HorusConfig: HorusConfig{
		//	HorusServerIP:   "10.211.104.23",
		//	HorusServerPort: 8383,
		//	HorusEventIP:    "10.211.104.23",
		//	HorusEventPort:  8484,
		},
		Registry: Registry{},
		SSHDeliver: SSHDeliver{
			SourceDir:       "./script/node-init",
			InitScriptName:  "node-init.sh",
			CleanScriptName: "node-clean.sh",
			CA_CRT_Name:     "registery-ca.crt",
			Destination:     "/tmp",
		},

		DockerPort: 2375,
		PluginPort: 0,
		Retry:      0,
	}
	t.Log(test.DestPath())
	t.Log(test.ConsulIPs, test.GetConsulAddrs())
	t.Log(test.GetConsulConfig())

	id, err := test.Insert()
	if err != nil {
		t.Fatal(err)
	}

	defer deleteSystemConfig(id)

	config, err := GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}

	t.Log(config.GetConsulConfig())
	t.Log(config.GetConsulAddrs())
	t.Log(config.DestPath())
}
