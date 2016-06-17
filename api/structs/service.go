package structs

import (
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
)

type PostServiceRequest struct {
	Name         string
	Description  string `json:",omitempty"`
	Architecture string `json:"arch"`
	BusinessCode string `business_code`

	AutoHealing   bool `json:",omitempty"`
	AutoScaling   bool `json:",omitempty"`
	HighAvailable bool `json:",omitempty"`

	Modules         []Module
	Users           []User          `json:",omitempty"`
	BackupRetention int             `json:"backup_retention"` // day
	BackupMaxSize   int             `json:"backup_max_size"`  // byte
	BackupStrategy  *BackupStrategy `json:"backup_strategy,omitempty"`
}

type User struct {
	Type      string // db/proxy
	Username  string
	Password  string
	Role      string
	Whitelist []string `json:",omitempty"`
}

type Module struct {
	Name       string
	Version    string
	Type       string                 // upsql\upproxy\sm
	Arch       string                 `json:"arch"`
	Candidates []string               `json:",omitempty"`
	Stores     []DiskStorage          `json:",omitempty"`
	Configures map[string]interface{} `json:",omitempty"`

	Config           container.Config         `json:",omitempty"`
	HostConfig       container.HostConfig     `json:"host_config",omitempty"`
	NetworkingConfig network.NetworkingConfig `json:"-"`
}

type DiskStorage struct {
	Name string // DAT/LOG/CNF
	Type string // "local:HDD/SSD,san"
	Size int
}

type BackupStrategy struct {
	Name      string
	Type      string // full/incremental
	Spec      string // cron spec
	Valid     string // "2006-01-02 15:04:05"
	BackupDir string `json:",omitempty"`
	Timeout   int    `json:",omitempty"` // xx Sec
	Enable    bool   // using in response
	CreatedAt string // using in response
}

type PostServiceResponse struct {
	ID               string
	TaskID           string
	BackupStrategyID string `json:"backup_strategy_id"`
}

type ScaleUpModule struct {
	Type   string
	Config container.UpdateConfig
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

		if config.RestartPolicy != (container.RestartPolicy{}) {
			req.Modules[i].HostConfig.RestartPolicy = config.RestartPolicy
		}

		break
	}
}

func (req *PostServiceRequest) UpdateModuleStore(list []StorageExtension) {
	for l := range list {
		for m := range req.Modules {
			if req.Modules[m].Type != list[l].Type {
				continue
			}

			for e := range list[l].Extensions {
				for i := range req.Modules[m].Stores {
					if list[l].Extensions[e].Name != req.Modules[m].Stores[i].Name {
						continue
					}

					req.Modules[m].Stores[i].Size += list[l].Extensions[e].Size
					break
				}

			}

			break
		}
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
	BackupFilesRetention int    `json:"backup_files_retention"`
	CreatedAt            string `json:"created_at"`
	FinishedAt           string `json:"finished_at"`

	Containers []UnitInfo `json:"containers"`
}

type UnitInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`    // <unit_id_8bit>_<service_name>
	Type     string `json:"type"`    // switch_manager/upproxy/upsql
	EngineID string `json:"node_id"` // engine.ID
	// ImageID		string `json:"image_id"`
	// ImageName	string `json:"image_name"` //<image_name>_<image_version>
	// ServiceID	string `json:"service_id"`
	// ContainerID	string `json:"container_id"`
	// ConfigID		string `json:"unit_config_id"`
	// NetworkMode	string `json:"network_mode"`

	Status        uint32 `json:"status"`
	CheckInterval int    `json:"check_interval"`
	CreatedAt     string `json:"created_at"`
	Info          string `json:"info"`
}
