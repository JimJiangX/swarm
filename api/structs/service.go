package structs

import (
	"time"

	"github.com/samalba/dockerclient"
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
	Type    string
	Arch    string // split by `-` ,"nMaster-mStandby-xSlave"
	Num     int

	Nodes      []string               `json:",omitempty"`
	Stores     []DiskStorage          `json:",omitempty"`
	Configures map[string]interface{} `json:",omitempty"`

	Config dockerclient.ContainerConfig
}

type DiskStorage struct {
	Name string // DATA / LOG
	Type string // "local\ssd\san"
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
