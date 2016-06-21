package swarm

import (
	"encoding/json"
	"errors"

	"github.com/Sirupsen/logrus"

	"github.com/hashicorp/consul/api"
)

var ErrConsulClientIsNil = errors.New("consul client is nil")

func HealthChecksFromConsul(client *api.Client, state string, q *api.QueryOptions) (map[string]api.HealthCheck, error) {
	if client == nil {
		return nil, ErrConsulClientIsNil
	}
	checks, _, err := client.Health().State(state, q)
	if err != nil {
		return nil, err
	}

	m := make(map[string]api.HealthCheck, len(checks))
	for _, val := range checks {
		m[val.ServiceID] = *val
	}

	return m, nil
}

func GetUnitRoleFromConsul(client *api.Client, service, unit string) (map[string]string, error) {
	if client == nil {
		return nil, ErrConsulClientIsNil
	}

	key := service + "/" + unit
	val, _, err := client.KV().Get(key, nil)
	if err != nil {
		logrus.Error(err, key)
		return nil, err
	}

	return rolesJSONUnmarshal(val.Value)
}

func rolesJSONUnmarshal(data []byte) (map[string]string, error) {
	roles := struct {
		Units struct {
			Default map[string]struct {
				Type string
			}
		} `json:"datanode_group"`
	}{}

	src := string(data)
	logrus.Debug("src: ", src)

	err := json.Unmarshal(data, &roles)
	if err != nil {
		logrus.Error(err, src)
		return nil, err
	}

	m := make(map[string]string, len(roles.Units.Default))
	for key, val := range roles.Units.Default {
		m[key] = val.Type
	}

	return m, nil
}
