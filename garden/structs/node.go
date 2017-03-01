package structs

type PostClusterRequest struct {
	MaxNode    int     `json:"max_node"`
	UsageLimit float32 `json:"usage_limit"`
}

type GetClusterResponse struct {
	ID         string
	MaxNode    int     `json:"max_node"`
	NodeNum    int     `json:"node_num"`
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
	Addr   string
	TaskID string
}
