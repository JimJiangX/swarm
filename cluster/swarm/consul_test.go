package swarm

import (
	"encoding/json"
	"testing"
)

func TestUnitRoleJsonUnmarshal(t *testing.T) {
	value := []byte(`
	{"version":"1","proxy_mode":{"datanode":"default"},"proxy_users":{},"database_auth":{"database_users":{"ap":{"password":"111111","omitempty":"ap"},"cup_dba":{"password":"111111","omitempty":"cup_dba"},"db":{"password":"111111","omitempty":"db"},"mon":{"password":"123.com","omitempty":"mon"},"repl":{"password":"111111","omitempty":"repl"}},"proxy_database_user_map":{}},"datanode_group":{"default":{"b8e1476c_jinrong01_01":{"ip":"192.168.20.102","port":30004,"status":"normal","type":"master"}}},"datanode_group_normal_count":{"default":1}}
	`)
	roles := struct {
		Units struct {
			Default map[string]struct {
				Type string
			}
		} `json:"datanode_group"`
	}{}

	err := json.Unmarshal(value, &roles)
	if err != nil {
		t.Error(err)
	}

	t.Logf("%v", roles)

	m := make(map[string]string, len(roles.Units.Default))
	for key, val := range roles.Units.Default {
		m[key] = val.Type
	}

	t.Logf("%v", m)
}
