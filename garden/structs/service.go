package structs

import (
	"github.com/docker/swarm/garden/database"
)

type VolumeRequire struct {
	From    string
	Name    string
	Type    string
	Driver  string
	Size    int64
	Options map[string]interface{}
}

type UnitSpec struct {
	database.Unit

	Require, Limit struct {
		CPU    string
		Memory int64
	}

	Engine struct {
		ID   string
		Name string
		IP   string
	}

	Networking struct {
		Type    string
		Devices string
		Mask    int
		IPs     []struct {
			Name  string
			IP    string
			Proto string
		}
		Ports []struct {
			Name string
			Port int
		}
	}

	Volumes []struct {
		Type    string
		Driver  string
		Size    int
		Options map[string]interface{}
	}
}

type ServiceSpec struct {
	Priority int
	Replicas int
	database.Service
	ContainerSpec ContainerSpec

	Constraint []string
	Options    map[string]interface{}

	Users []database.User

	Units []UnitSpec

	Deps []*ServiceSpec
}

type ContainerSpec struct {
	Require, Limit struct {
		CPU    string
		Memory int64
	}

	Volumes []VolumeRequire
}

type RequireResource struct {
	IPs []struct {
		Name  string
		IP    string
		Proto string
	}
	Ports []struct {
		Name string
		Port int
	}
}

type PostServiceResponse []database.Service
