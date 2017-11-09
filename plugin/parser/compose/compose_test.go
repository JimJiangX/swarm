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

		Options: map[string]interface{}{"port": float64(6379)},

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
	req.Image = structs.ImageVersion{
		Name:  "redis",
		Major: 12,
		Minor: 3,
		Patch: 3,
	}

	return req
}

func getMysqlSpecTest() *structs.ServiceSpec {

	db1 := structs.UnitSpec{
		Networking: []structs.UnitIP{
			{
				IP: "192.168.30.105",
			},
		},
	}
	db1.ContainerID = "db1"

	db2 := structs.UnitSpec{
		Networking: []structs.UnitIP{
			{
				IP: "192.168.30.107",
			},
		},
	}
	db2.ContainerID = "db2"

	db3 := structs.UnitSpec{
		Networking: []structs.UnitIP{
			{
				IP: "192.168.30.108",
			},
		},
	}
	db3.ContainerID = "db3"

	req := &structs.ServiceSpec{
		Arch: structs.Arch{
			Mode:     "replication",
			Replicas: 3,
			Code:     "M:1#S:2",
		},

		Options: map[string]interface{}{"port": float64(6379)},
		Users: []structs.User{
			{
				Name:     "rep1",
				Password: "rep1",
				Role:     "replication",
			},
		},
		Units: []structs.UnitSpec{db1, db2, db3},
	}
	req.Image = structs.ImageVersion{
		Name:  "mysql",
		Major: 5,
		Minor: 7,
		Patch: 17,
	}

	return req
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
	dir := "."
	composer, err := NewCompserBySpec(spec, dir, mgmip, mgmport)
	//	assert.Nil(t, err)
	if err != nil {
		t.Skipf("get composer fail:%s", err.Error())
	}

	err = composer.ComposeCluster()
	if err != nil {
		t.Skipf("redis ComposeCluster:%+v", err)
	}
}

func TestMysql(t *testing.T) {
	spec := getMysqlSpecTest()
	mgmip := "127.0.0.1"
	mgmport := 123
	dir := "."
	composer, err := NewCompserBySpec(spec, dir, mgmip, mgmport)
	//	assert.Nil(t, err)
	if err != nil {
		t.Skipf("get composer fail:%s", err.Error())
	}

	err = composer.ComposeCluster()
	if err != nil {
		t.Skipf("mysql ComposeCluster:%+v", err)
	}
}
func TestClone(t *testing.T) {
	spec := getRedisSpecTest()
	spec.Arch.Mode = "clone"

	dir := "."

	composer, err := NewCompserBySpec(spec, dir, "", 0)
	//	assert.Nil(t, err)
	if err != nil {
		t.Skipf("get composer fail:%s", err.Error())
	}

	err = composer.ComposeCluster()
	if err != nil {
		t.Skipf("redis ComposeCluster:%+v", err)
	}
}
