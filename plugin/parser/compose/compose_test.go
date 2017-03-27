package compose

import (
	"testing"

	"github.com/docker/swarm/garden/structs"
	"github.com/stretchr/testify/assert"
)

func getRedisSpecTest() *structs.ServiceSpec {
	req := &structs.ServiceSpec{
		Arch: structs.Arch{
			Mode:     "sharding_replication",
			Replicas: 3,
			Code:     "m:3#s:0",
		},

		Units: []structs.UnitSpec{
			{
				Networking: structs.UnitNetworking{
					IPs: []structs.UnitIP{
						{
							IP: "192.168.4.141",
						},
					},
					Ports: []structs.UnitPort{
						{
							Port: 6379,
						},
					},
				},
			},

			{
				Networking: structs.UnitNetworking{
					IPs: []structs.UnitIP{
						{
							IP: "192.168.4.141",
						},
					},
					Ports: []structs.UnitPort{
						{
							Port: 6380,
						},
					},
				},
			},

			{
				Networking: structs.UnitNetworking{
					IPs: []structs.UnitIP{
						{
							IP: "192.168.4.141",
						},
					},
					Ports: []structs.UnitPort{
						{
							Port: 6381,
						},
					},
				},
			},
		},
	}
	req.Image = "redis:12.3.3"

	return req
}

func getMysqlSpecTest() *structs.ServiceSpec {
	return &structs.ServiceSpec{}
}

func TestRedis(t *testing.T) {
	spec := getRedisSpecTest()
	mgmip := "127.0.0.1"
	mgmport := 123
	composer, err := NewCompserBySpec(spec, mgmip, mgmport)
	//	assert.Nil(t, err)
	if err != nil {
		t.Fatalf("get composer fail:%s", err.Error())
	}

	assert.Nil(t, composer.ComposeCluster())

}
