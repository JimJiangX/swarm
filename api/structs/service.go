package structs

import (
	"time"

	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
)

type PostServiceRequest struct {
	Name         string
	Description  string `json:",omitempty"`
	Architecture string `json:"arch"`

	AutoHealing   bool `json:",omitempty"`
	AutoScaling   bool `json:",omitempty"`
	HighAvailable bool `json:",omitempty"`

	Modules         []Module
	Users           []User          `json:",omitempty"`
	BackupRetention time.Duration   `json:"backup_retention"` // s
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
	Type      string        // full/incremental
	Spec      string        // cron spec
	Valid     string        // "2006-01-02 15:04:05"
	BackupDir string        `json:",omitempty"`
	Timeout   time.Duration // xx Sec
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

type Candidates struct {
	Candidates []string
}
