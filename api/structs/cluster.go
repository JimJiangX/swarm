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

// ListClusterResource used for GET: /clusters Response Body structure
type ListClusterResource []ClusterResource

type ClusterResource struct {
	ID    string
	Name  string
	Total Resource
	Nodes []NodeResource
}

type Resource struct {
	TotalCPU    int
	FreeCPU     int
	TotalMemory int
	FreeMemory  int
}

type NodeResource struct {
	ID     string
	Name   string
	Addr   string
	Status string
	Resource
}

// NodeInfo used for GET: /clusters/{name:.*}/nodes/{node:.*} Resonse structure
type NodeResourceInfo struct {
	NodeResource
	Containers []types.ContainerNode
}

// ClusterInspect used for GET: /clusters/{name:.*} Response structure
type ClusterResourceInspect struct {
	ID    string
	Name  string
	Nodes []NodeResourceInfo
}

type Node struct {
	Name     string
	Address  string
	Username string
	Password string
	HDD      []string `json:"hdd"`
	SSD      []string `json:"ssd"`

	Port         int `json:",omitempty"` // ssh port
	MaxContainer int `json:"max_container,omitempty"`

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
	ID           string `json:"id"`
	Name         string `json:"name"`
	ClusterID    string `json:"cluster_id"`
	Addr         string `json:"admin_ip"`
	EngineID     string `json:"engine_id"`
	DockerStatus string `json:"docker_status"`
	Room         string `json:"room"`
	Seat         string `json:"seat"`
	MaxContainer int    `json:"max_container"`
	Status       int    `json:"status"`
	RegisterAt   string `json:"register_at"`
}
