package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func getTestRedis() []Redis {

	return []Redis{
		Redis{
			Ip:   "192.168.4.141",
			Port: 6379,
		},
		Redis{
			Ip:   "192.168.4.141",
			Port: 6381,
		},
		Redis{
			Ip:   "192.168.4.141",
			Port: 6380,
		},
	}
}

func TestMysqlMS(t *testing.T) {
	datas := getTestRedis()
	master := 1
	slave := 0
	composer := newRedisShadeManager(datas, master, slave)

	assert.Nil(t, composer.ComposeCluster())
}
