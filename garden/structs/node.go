package structs

type PostClusterRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`

	MaxNode    int     `json:"max_node"`
	UsageLimit float32 `json:"usage_limit"`
}

type Node struct {
	Name    string
	Address string

	SSHConfig

	HDD []string `json:"hdd"`
	SSD []string `json:"ssd"`

	MaxContainer int `json:"max_container"`

	Room string `json:",omitempty"`
	Seat string `json:",omitempty"`
}

type SSHConfig struct {
	Username string
	Password string
	Port     int `json:",omitempty"` // ssh port
}

type PostNodesRequest []Node

type PostNodeResponse struct {
	ID     string
	Name   string
	TaskID string
}
