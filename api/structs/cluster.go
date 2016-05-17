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

// ListClusterResource used for GET: /clusters Response Body structure
type ListClusterResource []ClusterResource

type ClusterResource struct {
	ID    string
	Name  string
	Total Resource
	Nodes []NodeResource `json:",omitempty"`
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
type NodeInfo struct {
	NodeResource
	Containers []types.ContainerNode
}

// ClusterInspect used for GET: /clusters/{name:.*} Response structure
type ClusterInspect struct {
	ID    string
	Name  string
	Nodes []NodeInfo
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
	StorageID   string  `json:"storage_id,omitempty"`
	Datacenter  string  `json:"dc"`
	Enabled     bool    `json:"enabled"`
	MaxNode     int     `json:"max_node"`
	NodeNum     int     `json:"node_num"`
	UsageLimit  float32 `json:"usage_limit"`
}
