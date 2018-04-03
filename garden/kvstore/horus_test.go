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
			ServiceName: "HS-127.0.0.1:8000",
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
	const state = `[{"Node":"D0206004","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":170084,"ModifyIndex":170084},{"Node":"D0206004","CheckID":"service:dcd371cb37a1cc23f791d645a9be7ddc:Docker","Name":"Service 'dcd371cb37a1cc23f791d645a9be7ddc:Docker' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.26:2375: Success","ServiceID":"dcd371cb37a1cc23f791d645a9be7ddc:Docker","ServiceName":"dcd371cb37a1cc23f791d645a9be7ddc:Docker","ServiceTags":[],"Definition":{},"CreateIndex":170100,"ModifyIndex":170125},{"Node":"D0206004","CheckID":"service:dcd371cb37a1cc23f791d645a9be7ddc:SwarmAgent","Name":"Service 'dcd371cb37a1cc23f791d645a9be7ddc:SwarmAgent' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.26:4123: Success","ServiceID":"dcd371cb37a1cc23f791d645a9be7ddc:SwarmAgent","ServiceName":"dcd371cb37a1cc23f791d645a9be7ddc:SwarmAgent","ServiceTags":[],"Definition":{},"CreateIndex":170104,"ModifyIndex":170126},{"Node":"D0206005","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":170086,"ModifyIndex":170086},{"Node":"D0206005","CheckID":"service:074399daba07ebbf179d41788aebe935:Docker","Name":"Service '074399daba07ebbf179d41788aebe935:Docker' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.27:2375: Success","ServiceID":"074399daba07ebbf179d41788aebe935:Docker","ServiceName":"074399daba07ebbf179d41788aebe935:Docker","ServiceTags":[],"Definition":{},"CreateIndex":170107,"ModifyIndex":170118},{"Node":"D0206005","CheckID":"service:074399daba07ebbf179d41788aebe935:SwarmAgent","Name":"Service '074399daba07ebbf179d41788aebe935:SwarmAgent' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.27:4123: Success","ServiceID":"074399daba07ebbf179d41788aebe935:SwarmAgent","ServiceName":"074399daba07ebbf179d41788aebe935:SwarmAgent","ServiceTags":[],"Definition":{},"CreateIndex":170110,"ModifyIndex":170117},{"Node":"D0206009","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":170089,"ModifyIndex":170089},{"Node":"D0206009","CheckID":"service:595681e55f0f0231b893bf4dfbbda055:Docker","Name":"Service '595681e55f0f0231b893bf4dfbbda055:Docker' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.31:2375: Success","ServiceID":"595681e55f0f0231b893bf4dfbbda055:Docker","ServiceName":"595681e55f0f0231b893bf4dfbbda055:Docker","ServiceTags":[],"Definition":{},"CreateIndex":170112,"ModifyIndex":170128},{"Node":"D0206009","CheckID":"service:595681e55f0f0231b893bf4dfbbda055:SwarmAgent","Name":"Service '595681e55f0f0231b893bf4dfbbda055:SwarmAgent' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.31:4123: Success","ServiceID":"595681e55f0f0231b893bf4dfbbda055:SwarmAgent","ServiceName":"595681e55f0f0231b893bf4dfbbda055:SwarmAgent","ServiceTags":[],"Definition":{},"CreateIndex":170115,"ModifyIndex":170129},{"Node":"D0206010","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":170092,"ModifyIndex":170092},{"Node":"D0206010","CheckID":"service:8a62d6e3614123a4c6e74c394edf27d8:Docker","Name":"Service '8a62d6e3614123a4c6e74c394edf27d8:Docker' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.32:2375: Success","ServiceID":"8a62d6e3614123a4c6e74c394edf27d8:Docker","ServiceName":"8a62d6e3614123a4c6e74c394edf27d8:Docker","ServiceTags":[],"Definition":{},"CreateIndex":170119,"ModifyIndex":170131},{"Node":"D0206010","CheckID":"service:8a62d6e3614123a4c6e74c394edf27d8:SwarmAgent","Name":"Service '8a62d6e3614123a4c6e74c394edf27d8:SwarmAgent' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.20.32:4123: Success","ServiceID":"8a62d6e3614123a4c6e74c394edf27d8:SwarmAgent","ServiceName":"8a62d6e3614123a4c6e74c394edf27d8:SwarmAgent","ServiceTags":[],"Definition":{},"CreateIndex":170122,"ModifyIndex":170124},{"Node":"MDBSCOU11","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":6,"ModifyIndex":6},{"Node":"MDBSCOU11","CheckID":"service:HS-146.32.100.11:20154","Name":"Service 'HS-146.32.100.11:20154' check","Status":"passing","Notes":"","Output":"TCP connect 146.32.100.11:20154: Success","ServiceID":"HS-146.32.100.11:20154","ServiceName":"HS-146.32.100.11:20154","ServiceTags":[],"Definition":{},"CreateIndex":265,"ModifyIndex":70614},{"Node":"MDBSCOU11","CheckID":"service:plymer-146.32.100.11:20155","Name":"Service 'plymer-146.32.100.11:20155' check","Status":"passing","Notes":"","Output":"TCP connect 146.32.100.11:20155: Success","ServiceID":"plymer-146.32.100.11:20155","ServiceName":"plymer-146.32.100.11:20155","ServiceTags":["plymer"],"Definition":{},"CreateIndex":1203,"ModifyIndex":168499},{"Node":"MDBSCOU12","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":7,"ModifyIndex":7},{"Node":"MDBSCOU13","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":5,"ModifyIndex":5},{"Node":"MDBSREG11","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":1331,"ModifyIndex":1331},{"Node":"ptdbsmgm01","CheckID":"serfHealth","Name":"Serf Health Status","Status":"passing","Notes":"","Output":"Agent alive and reachable","ServiceID":"","ServiceName":"","ServiceTags":[],"Definition":{},"CreateIndex":63,"ModifyIndex":600181},{"Node":"ptdbsmgm01","CheckID":"service:MG-146.33.33.12:20152","Name":"Service 'MG-146.33.33.12:20152' check","Status":"passing","Notes":"","Output":"TCP connect 146.33.33.12:20152: Success","ServiceID":"MG-146.33.33.12:20152","ServiceName":"MG-146.33.33.12:20152","ServiceTags":[],"Definition":{},"CreateIndex":1987,"ModifyIndex":612546}]`
	var checks api.HealthChecks
	err := json.Unmarshal([]byte(state), &checks)
	if err != nil {
		t.Error(err)
	}

	ok := false

	for i := range checks {
		addr := parseIPFromHealthCheck(checks[i].ServiceName, checks[i].Output)
		if addr != "" {
			ok = true
			t.Log(addr)
		}
	}

	if !ok {
		t.Error("non-available Horus query from KV store")
	}
}
