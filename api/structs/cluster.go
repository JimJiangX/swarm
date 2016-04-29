package structs

import (
	"strings"

	"github.com/docker/engine-api/types"
)

type PostClusterRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Datacenter string `json:"dc"`

	MaxNode    int     `json:"max_node"`
	UsageLimit float32 `json:"usage_limit"`

	StorageType string `json:"storage_type"`
	StorageID   string `json:"storage_id,omitempty"`
}

func (req PostClusterRequest) Validate() string {
	warnings := make([]string, 0, 5)
	if req.Name == "" {
		warnings = append(warnings, "'name' is null")
	}

	if !strings.EqualFold(req.Type, "upsql") && !strings.EqualFold(req.Type, "proxy") {
		warnings = append(warnings, "'type' is either 'upsql' or 'proxy'")
	}

	if req.Datacenter == "" {
		warnings = append(warnings, "'dc' is null")
	}

	if len(warnings) == 0 {
		return ""
	}

	return strings.Join(warnings, ",")
}

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

type NodeRespect struct {
	NodeResource
	Containers []types.ContainerJSONBase
}

type Node struct {
	Name     string
	Address  string
	Username string
	Password string
	HDD      string `json:"hdd"`
	SSD      string `json:"ssd"`

	Port         int `json:",omitempty"` // ssh port
	MaxContainer int `json:"max_container,omitempty"`
}

type PostNodesRequest []Node

type PostNodeResponse struct {
	ID     string
	Name   string
	TaskID string
}
