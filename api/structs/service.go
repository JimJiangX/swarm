package structs

import (
	"time"

	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
)

type PostServiceRequest struct {
	Name         string
	Description  string `json:",omitempty"`
	Architecture string

	AutoHealing   bool `json:",omitempty"`
	AutoScaling   bool `json:",omitempty"`
	HighAvailable bool `json:",omitempty"`

	Modules  []Module
	Users    []User         `json:",omitempty"`
	Strategy BackupStrategy `json:",omitempty"`
}

type User struct {
	Type      string // db/proxy
	Username  string
	Password  string
	Role      string
	Whitelist []string `json:",omitempty"`
}

type Module struct {
	Name    string
	Version string
	Type    string // upsql\upproxy\sm
	Arch    string // split by `-` ,"nMaster-mStandby-xSlave"
	Num     int

	Nodes      []string               `json:",omitempty"`
	Stores     []DiskStorage          `json:",omitempty"`
	Configures map[string]interface{} `json:",omitempty"`

	Config           container.Config
	HostConfig       container.HostConfig
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
