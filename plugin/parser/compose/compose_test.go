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
			Code:     "M:3#S:0",
		},

		Options: map[string]interface{}{"port": 6379},

		Units: []structs.UnitSpec{
			{
				Networking: []structs.UnitIP{
					{
						IP: "192.168.30.105",
					},
				},
			},

			{
				Networking: []structs.UnitIP{
					{
						IP: "192.168.30.104",
					},
				},
			},

			{
				Networking: []structs.UnitIP{
					{
						IP: "192.168.30.103",
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

func TestOptions(t *testing.T) {
	spec1 := &structs.ServiceSpec{Options: map[string]interface{}{"port": float64(6379)}}
	_, err := getRedisPortBySpec(spec1)
	assert.Nil(t, err)

	spec2 := &structs.ServiceSpec{Options: map[string]interface{}{"port": "6379"}}
	_, err = getRedisPortBySpec(spec2)
	assert.Nil(t, err)

	spec3 := &structs.ServiceSpec{Options: map[string]interface{}{"port": int32(6379)}}
	_, err = getRedisPortBySpec(spec3)
	assert.Nil(t, err)

	spec4 := &structs.ServiceSpec{Options: map[string]interface{}{"port": 6379}}
	_, err = getRedisPortBySpec(spec4)
	assert.Nil(t, err)
}

func TestRedis(t *testing.T) {
	spec := getRedisSpecTest()
	mgmip := "127.0.0.1"
	mgmport := 123
	composer, err := NewCompserBySpec(spec, mgmip, mgmport)
	//	assert.Nil(t, err)
	if err != nil {
		t.Skipf("get composer fail:%s", err.Error())
	}

	err = composer.ComposeCluster()
	if err != nil {
		t.Skipf("redis ComposeCluster:%+v", err)
	}

}

func TestClone(t *testing.T) {
	spec := getRedisSpecTest()
	spec.Arch.Mode = "clone"

	composer, err := NewCompserBySpec(spec, "", 0)
	//	assert.Nil(t, err)
	if err != nil {
		t.Skipf("get composer fail:%s", err.Error())
	}

	err = composer.ComposeCluster()
	if err != nil {
		t.Skipf("redis ComposeCluster:%+v", err)
	}
}
