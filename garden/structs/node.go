package structs

type PostClusterRequest struct {
	Max        int     `json:"max_host"`
	UsageLimit float32 `json:"usage_limit"`
}

type GetClusterResponse struct {
	ID         string  `json:"id"`
	MaxNode    int     `json:"max_host"`
	NodeNum    int     `json:"host_num"`
	UsageLimit float32 `json:"usage_limit"`
}

type Node struct {
	Cluster string `json:"cluster_id"`
	Addr    string `json:"addr"`

	SSHConfig
	NFS

	HDD []string `json:"hdd"`
	SSD []string `json:"ssd"`

	MaxContainer int `json:"max_container"`

	Room string `json:"room,omitempty"`
	Seat string `json:"seat,omitempty"`
}

type NFS struct {
	Address  string `json:"nfs_ip"`
	Dir      string `json:"nfs_dir"`
	MountDir string `json:"nfs_mount_dir"`
	Options  string `json:"nfs_mount_opts"`
}

type SSHConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Port     int    `json:"port,omitempty"` // ssh port
}

type PostNodesRequest []Node

type PostNodeResponse struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
	Task string `json:"task_id"`
}
