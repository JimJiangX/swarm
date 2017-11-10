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
			{
				ID: "upredisID0001",
				Spec: &structs.ServiceSpec{
					Service: structs.Service{
						ID:   "upredisID0001",
						Name: "upredisName0001",
						Image: structs.ImageVersion{
							Name:  "upredis",
							Major: 1,
							Minor: 1,
							Patch: 5,
						},
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
			{
				ID: "urproxyID0001",
				Spec: &structs.ServiceSpec{
					Service: structs.Service{
						ID:   "urproxyID0001",
						Name: "urproxyName0001",
						Image: structs.ImageVersion{
							Name:  "urproxy",
							Major: 1,
							Minor: 1,
							Patch: 5,
						},
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
			{
				ID: "sentinelID0001",
				Spec: &structs.ServiceSpec{
					Service: structs.Service{
						ID:   "sentinelID0001",
						Name: "sentinelName0001",
						Image: structs.ImageVersion{
							Name:  "sentinel",
							Major: 1,
							Minor: 1,
							Patch: 5,
						},
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
