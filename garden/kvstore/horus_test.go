package kvstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/docker/swarm/garden/structs"
	"github.com/hashicorp/consul/api"
	"golang.org/x/net/context"
)

func init() {
	r := http.NewServeMux()
	r.HandleFunc("/v1/status/leader", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode("127.0.0.1:6060")
	})

	r.HandleFunc("/v1/status/peers", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]string{"127.0.0.1:6060"})
	})

	r.HandleFunc("/v1/agent/service/register", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.HandleFunc("/v1/health/state/", func(w http.ResponseWriter, r *http.Request) {
		ck := api.HealthCheck{
			Output:      "TCP connect 127.0.0.1:8000: Success",
			ServiceName: "HS-127.0.0.1",
		}
		checks := []*api.HealthCheck{&ck}
		json.NewEncoder(w).Encode(checks)
	})

	go http.ListenAndServe(":6060", r)

	mockRegisterServer()
}

func mockRegisterServer() {
	r := http.NewServeMux()

	r.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {

		fmt.Println(r.Method, r.RequestURI)

		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		} else if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})

	go http.ListenAndServe(":8000", r)
}

func makeClient() (*kvClient, error) {
	return MakeClient(&api.Config{
		Address: "127.0.0.1:6060",
	}, "prefix", "6060", nil)
}

func TestGetHorusAddr(t *testing.T) {
	c, err := makeClient()
	if err != nil {
		t.Skip(err)
	}

	leader := c.getLeader()
	if leader == "" {
		t.Error("Unexpected")
	} else {
		t.Log("leader:", leader)
	}

	addr, err := c.GetHorusAddr(nil)
	if err != nil {
		t.Error(err)
	}

	t.Log("horus:", addr)
}

func TestRegisterService(t *testing.T) {
	c, err := makeClient()
	if err != nil {
		t.Skip(err)
	}

	config := structs.ServiceRegistration{
		Consul: &api.AgentServiceRegistration{},
		Horus:  &structs.HorusRegistration{},
	}

	config.Horus.Node.Select = true
	config.Horus.Service.Select = true

	err = c.RegisterService(context.Background(), "", config)
	if err != nil {
		t.Errorf("%+v", err)
	}
}

func TestDeregisterService(t *testing.T) {
	c, err := makeClient()
	if err != nil {
		t.Skip(err)
	}

	configs := []structs.ServiceDeregistration{
		{
			Type: unitType,
			Key:  "unit_jfajfoafajofjaof",
		}, {
			Type: containerType,
			Key:  "container_jfajfoafajofjaof",
		}, {
			Type:     hostType,
			Key:      "host_jfajfoafajofjaof",
			User:     "foiafjoafoia",
			Password: "ofjajfioafoaf",
		},
	}

	for i := range configs {
		c.DeregisterService(context.Background(), configs[i], false)
		if err != nil {
			t.Errorf("%+v", err)
		}
	}
}

func TestParseIPFromHealthCheck(t *testing.T) {
	output := "TCP connect 192.168.4.123:8000: Success"
	id := "HS-192.168.4.123"

	addr := parseIPFromHealthCheck(id, output)
	if addr == "" {
		t.Error("Unexpected")
	}

	t.Logf("'%s'", addr)
}
