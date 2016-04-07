package structs

type PostClusterRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Datacenter string `json:"datacenter"`

	MaxNode    int     `json:"max_node"`
	UsageLimit float32 `json:"usage_limit"`

	StorageType string `json:"storage_type"`
	StorageID   string `json:"storage_id,omitempty"`
}
