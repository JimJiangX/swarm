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
	Cluster string `json:"cluster_id"`
	Addr    string `json:"addr"`

	SSHConfig
	NFS NFS

	HDD []string `json:"hdd"`
	SSD []string `json:"ssd"`

	MaxContainer int `json:"max_container"`

	Room string `json:",omitempty"`
	Seat string `json:",omitempty"`
}

type NFS struct {
	Addr     string `json:"nfs_ip"`
	Dir      string `json:"nfs_dir"`
	MountDir string `json:"nfs_mount_dir"`
	Options  string `json:"nfs_mount_opts"`
}

type SSHConfig struct {
	Username string
	Password string
	Port     int `json:",omitempty"` // ssh port
}

type PostNodesRequest []Node

type PostNodeResponse struct {
	ID   string
	Addr string
}
