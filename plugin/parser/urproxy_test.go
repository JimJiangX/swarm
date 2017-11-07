package parser

import (
	"testing"

	"github.com/docker/swarm/garden/structs"
)

var urproxyTemplate = structs.ConfigTemplate{
	Image: "urproxy:1.2.3.4",
	// Mount     string
	LogMount:   "/UPM/LOG",
	DataMount:  "/UPM/DAT",
	ConfigFile: "urproxy.conf",
	Content: `default:
  auto_eject_hosts: false
  distribution: modula
  hash: fnv1a_64
  listen: <IP>:<PORT>
  preconnect: true
  redis: true
  redis_auth: dbaas
  timeout: 4000
  server_connections: 1
  sentinels:
  - <S1>:<S_PORT>
  - <S2>:<S_PORT>
  - <S3>:<S_PORT>
  black_list:
  - 0.0.0.0`,
}

func TestUrproxyParser(t *testing.T) {

	pr, err := factory(urproxyTemplate.Image)
	if err != nil {
		t.Errorf("%+v", err)
	}

	pr = pr.clone(&urproxyTemplate)

	err = pr.ParseData([]byte(urproxyTemplate.Content))
	if err != nil {
		t.Errorf("%+v", err)
	}

	err = pr.set("white_list", "0.0.0.3&&&*.*.*.*&&&0.0.0.0")
	if err != nil {
		t.Errorf("%+v", err)
	}

	text, err := pr.Marshal()
	if err != nil {
		t.Errorf("%+v", err)
	}

	t.Logf("%s", text)
}
