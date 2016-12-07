package kvstore

import "testing"

func TestRolesJSONUnmarshal(t *testing.T) {
	value := []byte(`{"version":"1","proxy_mode":{"datanode":"default"},"proxy_users":{},"database_auth":{"database_users":{"ap":{"password":"111111","omitempty":"ap"},"cup_dba":{"password":"111111","omitempty":"cup_dba"},"db":{"password":"111111","omitempty":"db"},"mon":{"password":"123.com","omitempty":"mon"},"repl":{"password":"111111","omitempty":"repl"}},"proxy_database_user_map":{}},"datanode_group":{"default":{"b8e1476c_jinrong01_01":{"ip":"192.168.20.102","port":30004,"status":"normal","type":"master"}}},"datanode_group_normal_count":{"default":1}}`)

	m, err := rolesJSONUnmarshal(value)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%v", m)

	m, err = rolesJSONUnmarshal(nil)
	if err == nil {
		t.Error("Unexpected")
	}
	t.Logf("%v %v", m, err)
}

func TestParseIPFromHealthCheck(t *testing.T) {
	output := "TCP connect 192.168.2.123:8000: Success"
	id := "HS-192.168.2.123"

	addr := parseIPFromHealthCheck(id, output)
	if addr != "" {
		t.Log(addr)
	} else {
		t.Error("Unexpected")
	}
}
