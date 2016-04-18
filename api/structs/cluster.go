package structs

type PostClusterRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Datacenter string `json:"dc"`

	MaxNode    int     `json:"max_node"`
	UsageLimit float32 `json:"usage_limit"`

	StorageType string `json:"storage_type"`
	StorageID   string `json:"storage_id,omitempty"`
}

type Node struct {
	Name     string
	Address  string
	Username string
	Password string
	HDD      string `json:"hdd"`
	SSD      string `json:"ssd"`

	Port         int `json:",omitempty"`
	MaxContainer int `json:"max_container,omitempty"`
}

type PostNodesRequest []Node

type PostNodeResponse struct {
	ID     string
	Name   string
	TaskID string
}
