package structs

import (
	"time"

	"github.com/docker/swarm/cluster"
)

// Service if table structure
type Service struct {
	ID   string `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
	// TODO:maybe remove
	Image             string `db:"image_id" json:"image_id"`       // imageName:imageVersion
	Desc              string `db:"description" json:"description"` // short for Description
	Architecture      string `db:"architecture" json:"architecture"`
	Tag               string `db:"tag" json:"tag"` // part of business
	AutoHealing       bool   `db:"auto_healing" json:"auto_healing"`
	AutoScaling       bool   `db:"auto_scaling" json:"auto_scaling"`
	HighAvailable     bool   `db:"high_available" json:"high_available"`
	Status            int    `db:"status" json:"status"`
	BackupMaxSizeByte int    `db:"backup_max_size" json:"backup_max_size"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	BackupFilesRetention int       `db:"backup_files_retention" json:"backup_files_retention"`
	CreatedAt            time.Time `db:"created_at" json:"created_at"`
	FinishedAt           time.Time `db:"finished_at" json:"finished_at"`
}

type VolumeRequire struct {
	From    string
	Name    string
	Type    string
	Driver  string
	Size    int64
	Options map[string]interface{}
}

type Unit struct {
	ID          string `db:"id" json:"id"`
	Name        string `db:"name" json:"name"` // containerName <unit_id_8bit>_<service_name>
	Type        string `db:"type" json:"type"` // switch_manager/upproxy/upsql
	ServiceID   string `db:"service_id" json:"service_id"`
	EngineID    string `db:"engine_id" json:"engine_id"` // engine.ID
	ContainerID string `db:"container_id" json:"container_id"`
	NetworkMode string `db:"network_mode" json:"network_mode"`
	Networks    string `db:"networks_desc" json:"networks_desc"`
	LatestError string `db:"latest_error" json:"latest_error"`

	Status    int       `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type UnitSpec struct {
	Unit

	Config *cluster.ContainerConfig

	Engine struct {
		ID   string
		Addr string
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
	Priority     int    `json:"priority"`
	Replicas     int    `json:"replicas"`
	ImageVersion string `json:"image_version"`

	Service

	Require UnitRequire `json:"unit_require"`

	Clusters    []string `json:"clusters"`
	Constraints []string `json:"constraints"`

	Options map[string]interface{} `json:"options"`

	Units []UnitSpec `json:"units"`

	Deps []*ServiceSpec `json:"dependence"`
}

type UnitRequire struct {
	Require, Limit struct {
		CPU    int   `json:"ncpu"`
		Memory int64 `json:"memory"`
	}

	Volumes []VolumeRequire `json:"volumes"`

	Networkings []NetDeviceRequire `json:"networks"`
}

type NetDeviceRequire struct {
	Device     int    `json:"device"`
	Bandwidth  int    `json:"bandwidth"` // M/s
	Networking string `json:"netwroking"`
}

type PostServiceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	TaskID string `json:"task_id"`
}

type RequireResource struct{}
