package structs

import (
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/swarm/cluster"
)

// Service if table structure
type Service struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Image         ImageVersion `json:"image"`
	Desc          string       `json:"description"` // short for Description
	Architecture  string       `json:"architecture"`
	Tag           string       `json:"tag"` // part of business
	AutoHealing   bool         `json:"auto_healing"`
	AutoScaling   bool         `json:"auto_scaling"`
	HighAvailable bool         `json:"high_available"`
	Status        int          `json:"status"`
	CreatedAt     string       `json:"created_at"`
	FinishedAt    string       `json:"finished_at"`
}

type VolumeRequire struct {
	ID     string `json:"-"` // used by volume expansion,Volume ID
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
	LatestError string `db:"latest_error" json:"latest_error"`
	Status      int    `db:"status" json:"status"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type UnitIP struct {
	Prefix     int    `json:"prefix"`
	VLAN       int    `json:"vlan"`
	Bandwidth  int    `json:"bandwidth"`
	Device     string `json:"device"`
	Name       string `json:"name"`
	IP         string `json:"ip_addr"`
	Proto      string `json:"proto,omitempty"`
	Gateway    string `json:"gateway"`
	Networking string `json:"networking_id"`
}

type UnitPort struct {
	Name string `json:"name,omitempty"`
	Port int    `json:"port"`
}

type VolumeSpec struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Driver  string                 `json:"driver"`
	Size    int                    `json:"size"`
	Options map[string]interface{} `json:"options"`
}

type UnitSpec struct {
	Unit `json:"unit"`

	Container types.Container          `json:"container"`
	Config    *cluster.ContainerConfig `json:"container_config,omitempty"`

	Engine struct {
		ID   string `json:"id"`
		Node string `json:"node"`
		Name string `json:"name"`
		Addr string `json:"addr"`
	} `json:"engine"`

	Networking []UnitIP   `json:"networkings"`
	Ports      []UnitPort `json:"ports,omitempty"`

	Volumes []VolumeSpec `json:"volumes"`
}

type Arch struct {
	Replicas int    `json:"unit_num"`
	Mode     string `json:"mode"`
	Code     string `json:"code"`
}

type ServiceSpec struct {
	Service

	Arch Arch `json:"architecture"`

	Require *UnitRequire `json:"unit_require,omitempty"`

	Networkings map[string][]string `json:"networking,omitempty"`

	Clusters []string `json:"cluster_id,omitempty"`

	Constraints []string `json:"constraints,omitempty"`

	Units []UnitSpec `json:"units"`

	Users []User `json:"users,omitempty"`

	Options map[string]interface{} `json:"opts"`
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

type ServiceResponse struct {
	ServiceSpec
	BuckupFileSum int `json:"backup_file_sum"`
}

type UpdateUnitRequire struct {
	Require struct {
		CPU    *int64 `json:"ncpu,omitempty"`
		Memory *int64 `json:"memory,omitempty"`
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

type ServiceScaleRequest struct {
	Compose    bool                   `json:"compose"`
	Arch       Arch                   `json:"architecture"`
	Candidates []string               `json:"candidates,omitempty"`
	Options    map[string]interface{} `json:"opts"`
}

type ServiceScaleResponse struct {
	Task   string       `json:"task_id"`
	Add    []UnitNameID `json:"add_units,omitempty"`
	Remove []UnitNameID `json:"remove_units,omitempty"`
}

type UnitRebuildRequest PostUnitMigrate

type PostServiceResponse struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	TaskID string       `json:"task_id"`
	Units  []UnitNameID `json:"units"`
}

type UnitNameID struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	Name      string `json:"name"`
	Password  string `json:"password"`
	Role      string `json:"role"`
	Privilege string `json:"privilege"`
}

type ServiceExecConfig struct {
	Detach    bool     `json:"detach"`
	Container string   `json:"nameOrID"`
	Cmd       []string `json:"cmd"`
}

type ServiceBackupConfig struct {
	Container   string `json:"nameOrID"`
	Type        string `json:"type"`
	Tables      string `json:"tables"` // full or incremental or tables
	Remark      string `json:"remark"`
	Tag         string `json:"tag"`
	Detach      bool   `json:"detach"`
	BackupDir   string `json:"backup_dir"`
	MaxSizeByte int    `json:"max_backup_space"`
	// count by Day,used in swarm.BackupTaskCallback(),calculate BackupFile.Retention
	FilesRetention int `json:"backup_files_retention"`

	Cmd []string `json:"cmd,omitempty"`
}

type ServiceRestoreRequest struct {
	File  string   `json:"file"`
	Units []string `json:"units"`
}

type PostUnitMigrate struct {
	Compose    bool     `json:"compose"`
	NameOrID   string   `json:"nameOrID"`
	Candidates []string `json:"candidates,omitempty"`
}
