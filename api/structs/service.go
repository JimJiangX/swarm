package structs

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

type PostServiceRequest struct {
	ID           string `json:"-"`
	Name         string
	Description  string `json:",omitempty"`
	Architecture string `json:"arch"`
	BusinessCode string `json:"business_code"`

	AutoHealing bool `json:",omitempty"`
	AutoScaling bool `json:",omitempty"`

	Modules         []Module
	Users           []User          `json:",omitempty"`
	BackupRetention int             `json:"backup_retention"` // day
	BackupMaxSize   int             `json:"backup_max_size"`  // byte
	BackupStrategy  *BackupStrategy `json:"backup_strategy,omitempty"`
}

type User struct {
	ReadOnly bool   `json:"read_only"`
	Type     string // db/proxy
	Username string
	Password string
	Role     string

	Whitelist []string // []string
	Blacklist []string // []string
}

type Module struct {
	HighAvailable bool `json:"high_available"`
	Name          string
	Version       string
	Type          string                 // upsql\upproxy\sm
	Arch          string                 `json:"arch"`
	Clusters      []string               `json:",omitempty"`
	Stores        []DiskStorage          `json:",omitempty"`
	Configures    map[string]interface{} `json:",omitempty"`

	Config           container.Config         `json:"-"`
	HostConfig       ResourceRequired         `json:"host_config,omitempty"`
	NetworkingConfig network.NetworkingConfig `json:"-"`
}

type ResourceRequired struct {
	Memory     int64  // Memory limit (in bytes)
	CpusetCpus string // CpusetCpus 0-2, 0,1
}

type DiskStorage struct {
	Name string // DAT/LOG/CNF
	Type string // "local:HDD/SSD,san"
	Size int
}

type BackupStrategy struct {
	ID        string
	Name      string
	Type      string // full/incremental
	Spec      string // cron spec
	Valid     string // "2006-01-02 15:04:05"
	BackupDir string `json:"backup_dir,omitempty"`
	Timeout   int    `json:",omitempty"` // xx Sec
	Enable    bool   // using in response
	CreatedAt string // using in response
}

type PostServiceResponse struct {
	ID               string
	TaskID           string
	BackupStrategyID string `json:"backup_strategy_id"`
	Error            string
}

type ScaleUpModule struct {
	Type   string
	Config container.UpdateConfig
}

type PostServiceScaledRequest struct {
	ExtendBackup int
	Type         string
	UpdateConfig *container.UpdateConfig
	Extensions   []DiskStorage
}

func (req *PostServiceRequest) UpdateModuleConfig(_type string, config container.UpdateConfig) {
	for i := range req.Modules {
		if req.Modules[i].Type != _type {
			continue
		}

		if config.Memory != 0 {
			req.Modules[i].HostConfig.Memory = config.Memory
		}

		if config.CpusetCpus != "" {
			req.Modules[i].HostConfig.CpusetCpus = config.CpusetCpus
		}

		break
	}
}

func (req *PostServiceRequest) UpdateModuleStore(_type string, extensions []DiskStorage) {
	for m := range req.Modules {
		if req.Modules[m].Type != _type {
			continue
		}

		for e := range extensions {
			for i := range req.Modules[m].Stores {
				if extensions[e].Name != req.Modules[m].Stores[i].Name {
					continue
				}

				req.Modules[m].Stores[i].Size += extensions[e].Size
				break
			}
		}

		break
	}
}

type StorageExtension struct {
	Type       string
	Extensions []DiskStorage
}

type PostRebuildUnit struct {
	Candidates []string
	HostConfig *container.HostConfig
}

type PostMigrateUnit struct {
	Candidates []string
	HostConfig *container.HostConfig
}

type ServiceResponse struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Architecture string             `json:"architecture"`
	Description  PostServiceRequest `json:"description"`
	// AutoHealing	bool	`json:"auto_healing"`
	// AutoScaling	bool	`json:"auto_scaling"`
	HighAvailable        bool   `json:"high_available"`
	Status               int64  `json:"status"`
	BackupMaxSizeByte    int    `json:"backup_max_size"`
	BackupUsedSizeByte   int    `json:"backup_used_size"`
	BackupFilesRetention int    `json:"backup_files_retention"` // Day
	RunningStatus        string `json:"running_status"`
	CreatedAt            string `json:"created_at"`
	FinishedAt           string `json:"finished_at"`

	Containers []UnitInfo `json:"containers"`
}

type UnitInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`    // <unit_id_8bit>_<service_name>
	Type        string `json:"type"`    // switch_manager/upproxy/upsql
	NodeID      string `json:"node_id"` // Node.ID
	NodeAddr    string `json:"node_addr"`
	ClusterID   string `json:"cluster_id"`
	Networkings []struct {
		Type string
		Addr string
	}
	Ports []struct {
		Name string
		Port int
	}
	// ImageID		string `json:"image_id"`
	// ImageName	string `json:"image_name"` //<image_name>_<image_version>
	// ServiceID	string `json:"service_id"`
	// ContainerID	string `json:"container_id"`
	// ConfigID		string `json:"unit_config_id"`
	// NetworkMode	string `json:"network_mode"`
	Role       string `json:"role,omitempty"`
	CpusetCpus string
	Memory     int64
	TaskStatus int64  `json:"task_status"`
	State      string // container state
	Status     string // service status
	LatestMsg  string `json:"latest_msg"`
	CreatedAt  string `json:"created_at"`
	// CheckInterval int    `json:"check_interval"`

	Info types.ContainerJSON `json:",omitempty"`
}

type PostSlowlogRequest struct {
	Enable          bool
	NotUsingIndexes bool `json:"not_using_indexes,omitempty"`
	LongQueryTime   int  `json:"long_query_time,omitempty"`
}
