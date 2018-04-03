package kvstore

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pkg/errors"
)

func rolesJSONUnmarshal(data []byte) (map[string]string, error) {
	roles := struct {
		Units struct {
			Default map[string]struct {
				Type   string
				Status string
			}
		} `json:"datanode_group"`
	}{}

	err := json.Unmarshal(data, &roles)
	if err != nil {
		return nil, errors.Wrapf(err, "%s", data)
	}

	m := make(map[string]string, len(roles.Units.Default))
	for key, val := range roles.Units.Default {
		m[key] = fmt.Sprintf("%s(%s)", val.Type, val.Status)
	}

	return m, nil
}

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
