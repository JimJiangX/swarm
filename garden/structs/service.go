package structs

import (
	"time"

	"github.com/docker/swarm/cluster"
)

// Service if table structure
type Service struct {
	ID                string `db:"id" json:"id"`
	Name              string `db:"name" json:"name"`
	Image             string `json:"image_version"`
	Desc              string `db:"description" json:"description"` // short for Description
	Architecture      string `db:"architecture" json:"architecture"`
	Tag               string `db:"tag" json:"tag"` // part of business
	AutoHealing       bool   `db:"auto_healing" json:"auto_healing"`
	AutoScaling       bool   `db:"auto_scaling" json:"auto_scaling"`
	HighAvailable     bool   `db:"high_available" json:"high_available"`
	Status            int    `db:"status" json:"status"`
	BackupMaxSizeByte int    `db:"backup_max_size" json:"max_backup_space"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	BackupFilesRetention int    `db:"backup_files_retention" json:"backup_files_retention"`
	CreatedAt            string `db:"created_at" json:"created_at"`
	FinishedAt           string `db:"finished_at" json:"finished_at"`
}

type VolumeRequire struct {
	From   string `json:"from,omitempty"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Driver string `json:"driver,omitempty"`
	Size   int64  `json:"size"`

	Options map[string]interface{} `json:"options"`
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
	Status      int    `db:"status" json:"status"`

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
	Replicas int `json:"unit_num"`

	Service

	Require UnitRequire `json:"unit_require"`

	Networkings []string `json:"networking_id"`

	Clusters []string `json:"cluster_id"`

	Constraints []string `json:"constraints"`

	Units []UnitSpec `json:"units"`

	Options map[string]interface{} `json:"args"`
}

type UnitRequire struct {
	Require struct {
		CPU    int   `json:"ncpu"`
		Memory int64 `json:"memory"`
	} `json:"require"`

	Limit *struct {
		CPU    int   `json:"ncpu"`
		Memory int64 `json:"memory"`
	} `json:"limit,omitempty"`

	Volumes  []VolumeRequire    `json:"volumes"`
	Networks []NetDeviceRequire `json:"networks"`
}

type NetDeviceRequire struct {
	Device    int `json:"device,omitempty"`
	Bandwidth int `json:"bandwidth"` // M/s
}

type PostServiceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	TaskID string `json:"task_id"`
}

type RequireResource struct{}
