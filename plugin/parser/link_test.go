package parser

import (
	"context"
	"testing"

	"github.com/docker/swarm/garden/kvstore"
	"github.com/docker/swarm/garden/structs"
)

func TestLinkFactory(t *testing.T) {
	req := structs.ServicesLink{
		Mode: Proxy_Redis,
		Links: []*structs.ServiceLink{
			&structs.ServiceLink{
				ID: "upredisID0001",
				Spec: &structs.ServiceSpec{
					Service: structs.Service{
						ID:    "upredisID0001",
						Name:  "upredisName0001",
						Image: "upredis:1.1.5",
					},
					Units: []structs.UnitSpec{
						{
							Unit: structs.Unit{
								ID:          "upredisID0001_001",
								Name:        "upredisName0001_001",
								ContainerID: "container00001",
							},
							Networking: []structs.UnitIP{
								{IP: "192.168.1.1"},
							},
						},
					},
				},
			},
			&structs.ServiceLink{
				ID: "urproxyID0001",
				Spec: &structs.ServiceSpec{
					Service: structs.Service{
						ID:    "urproxyID0001",
						Name:  "urproxyName0001",
						Image: "urproxy:1.1.5",
					},
					Units: []structs.UnitSpec{
						{
							Unit: structs.Unit{
								ID:          "urproxyID0001_001",
								Name:        "urproxyName0001_001",
								ContainerID: "container00002",
							},
							Networking: []structs.UnitIP{
								{IP: "192.168.1.2"},
							},
						},
					},
				},
			},
			&structs.ServiceLink{
				ID: "sentinelID0001",
				Spec: &structs.ServiceSpec{
					Service: structs.Service{
						ID:    "sentinelID0001",
						Name:  "sentinelName0001",
						Image: "sentinel:1.1.5",
					},
					Units: []structs.UnitSpec{
						{
							Unit: structs.Unit{
								ID:          "sentinelID0001_001",
								Name:        "sentinelName0001_001",
								ContainerID: "container00003",
							},
							Networking: []structs.UnitIP{
								{IP: "192.168.1.3"},
							},
						},
					},
				},
			},
		},
	}

	lr, err := linkFactory(req.Mode, req.NameOrID, req.Links)
	if err != nil {
		t.Errorf("%+v", err)
	}

	_, err = lr.generateLinkConfig(context.Background(), kvstore.NewMockClient())
	if err == nil {
		t.Errorf("%+v", err)
	}
}
