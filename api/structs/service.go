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

	Modules        []Module
	Users          []User         `json:",omitempty"`
	BackupStrategy BackupStrategy `json:"backup_strategy,omitempty"`
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
	Nodes      []string               `json:",omitempty"`
	Stores     []DiskStorage          `json:",omitempty"`
	Configures map[string]interface{} `json:",omitempty"`

	Config           container.Config         `json:",omitempty"`
	HostConfig       container.HostConfig     `json:"host_config",omitempty"`
	NetworkingConfig network.NetworkingConfig `json:"-"`
}

type DiskStorage struct {
	Name string // DATA / LOG
	Type string // "local:HDD/SSD,san"
	Size int
}

type BackupStrategy struct {
	Type      string        // full/incremental
	Spec      string        // cron spec
	Valid     time.Time     // xxM-xxD-xxH-xxM-xxS
	Timeout   time.Duration // xx Sec
	Retention time.Duration // s
	MaxSize   int           // byte
	BackupDir string        `json:",omitempty"`
}

type PostServiceResponse struct {
	ID     string
	TaskID string
}
