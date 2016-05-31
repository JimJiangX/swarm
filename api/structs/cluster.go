package structs

import "github.com/docker/engine-api/types"

type PostClusterRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Datacenter string `json:"dc"`

	MaxNode    int     `json:"max_node"`
	UsageLimit float32 `json:"usage_limit"`

	StorageType string `json:"storage_type"`
	StorageID   string `json:"storage_id,omitempty"`
}

type UpdateClusterParamsRequest struct {
	MaxNode    int     `json:"max_node"`
	UsageLimit float32 `json:"usage_limit"`
}

type UpdateNodeSetting struct {
	MaxContainer int `json:"max_container"`
}

// ListClusterResource used for GET: /clusters/resources Response Body structure
type ListClusterResource []ClusterResource

type ClusterResource struct {
	Enable bool
	ID     string
	Name   string
	Entire Resource
	Nodes  []NodeResource `json:",omitempty"` // only used in GET /clusters/{name:.*}/resources
}

type Resource struct {
	TotalCPUs   int
	UsedCPUs    int
	TotalMemory int
	UsedMemory  int
}

type NodeResource struct {
	ID       string
	Name     string
	EngineID string
	Addr     string
	Status   string
	Resource
	Containers []ContainerWithResource
}

type ContainerWithResource struct {
	ID             string
	Name           string
	Image          string
	Driver         string
	NetworkMode    string
	Created        string
	State          string
	Labels         map[string]string
	Env            []string
	Mounts         []types.MountPoint
	CpusetCpus     string // CpusetCpus 0-2, 0,1
	CPUs           int64  // CPU shares (relative weight vs. other containers)
	Memory         int64  // Memory limit (in bytes)
	MemorySwap     int64  // Total memory usage (memory + swap); set `-1` to enable unlimited swap
	OomKillDisable bool   // Whether to disable OOM Killer or not
}

type Node struct {
	Name     string
	Address  string
	Username string
	Password string
	HDD      []string `json:"hdd"`
	SSD      []string `json:"ssd"`

	Port         int `json:",omitempty"` // ssh port
	MaxContainer int `json:"max_container"`

	Room string `json:",omitempty"`
	Seat string `json:",omitempty"`
}

type PostNodesRequest []Node

type PostNodeResponse struct {
	ID     string
	Name   string
	TaskID string
}

type ClusterInfoResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	StorageType string  `json:"storage_type"`
	StorageID   string  `json:"storage_id"`
	Datacenter  string  `json:"dc"`
	Enabled     bool    `json:"enabled"`
	MaxNode     int     `json:"max_node"`
	NodeNum     int     `json:"node_num"`
	UsageLimit  float32 `json:"usage_limit"`
}

type PerClusterInfoResponse struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	StorageType string        `json:"storage_type"`
	StorageID   string        `json:"storage_id"`
	Datacenter  string        `json:"dc"`
	Enabled     bool          `json:"enabled"`
	MaxNode     int           `json:"max_node"`
	UsageLimit  float32       `json:"usage_limit"`
	Nodes       []NodeInspect `json:"nodes"`
}

type NodeInspect struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ClusterID    string   `json:"cluster_id"`
	Addr         string   `json:"admin_ip"`
	EngineID     string   `json:"engine_id"`
	DockerStatus string   `json:"docker_status"`
	Room         string   `json:"room"`
	Seat         string   `json:"seat"`
	MaxContainer int      `json:"max_container"`
	Status       int      `json:"status"`
	RegisterAt   string   `json:"register_at"`
	Resource     Resource `json:"resource"`
}
