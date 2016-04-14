package database

import "testing"

func TestSystemConfig(t *testing.T) {
	config, err := GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}

	client, err := config.GetConsulClient()
	if err != nil || client == nil {
		t.Fatal(err)
	}

	t.Log(config.GetConsulConfig())
	t.Log(config.GetConsulAddrs())
	t.Log(config.DestPath())
}
