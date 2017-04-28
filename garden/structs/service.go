package structs

import (
	"sort"
	"time"

	"github.com/docker/docker/api/types"
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

	Networkings []string `json:"networking_id,omitempty"`

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

type NetDeviceRequire struct {
	Device    int `json:"device,omitempty"`
	Bandwidth int `json:"bandwidth"` // M/s
}

type ServiceScaleRequest struct {
	Arch Arch `json:"architecture"`

	Users []User `json:"users,omitempty"`

	Options map[string]interface{} `json:"opts"`
}

type PostServiceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	TaskID string `json:"task_id"`
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

type ServiceLink struct {
	priority int
	Spec     *ServiceSpec `json:"-"`

	ID   string   `json:"from_service_name"`
	Deps []string `json:"to_services_name"`
}

type ServicesLink []*ServiceLink

func (sl ServicesLink) Less(i, j int) bool {
	return sl[i].priority > sl[j].priority
}

// Len is the number of elements in the collection.
func (sl ServicesLink) Len() int {
	return len(sl)
}

// Swap swaps the elements with indexes i and j.
func (sl ServicesLink) Swap(i, j int) {
	sl[i], sl[j] = sl[j], sl[i]
}

// https://play.golang.org/p/1tkv9z4DtC
func (sl ServicesLink) Sort() {
	deps := make(map[string]int, len(sl))

	for i := range sl {
		deps[sl[i].ID] = len(sl[i].Deps)
	}

	for i := len(sl) - 1; i > 0; i-- {
		for _, s := range sl {

			max := 0

			for _, id := range s.Deps {
				if n := deps[id]; n > max {
					max = n + 1
				}
			}
			if max > 0 {
				deps[s.ID] = max
			}
		}
	}

	for i := range sl {
		sl[i].priority = deps[sl[i].ID]
	}

	sort.Sort(sl)
}

func (sl ServicesLink) Links() []string {
	l := make([]string, 0, len(sl)*2)
	for i := range sl {
		l = append(l, sl[i].ID)
		l = append(l, sl[i].Deps...)
	}

	ids := make([]string, 0, len(l))

	for i := range l {
		ok := false
		for c := range ids {
			if ids[c] == l[i] {
				ok = true
				break
			}
		}
		if !ok {
			ids = append(ids, l[i])
		}
	}

	return ids
}
