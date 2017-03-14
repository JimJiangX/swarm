package structs

type RegisterDC struct {
	ID             int          `json:"dc_id"`
	DockerPort     int          `json:"docker_port"`
	PluginPort     int          `json:"plugin_port"`
	SwarmAgentPort int          `json:"swarm_agent_port"`
	BackupDir      string       `json:"backup_dir"`
	Retry          int64        `json:"retry"`
	NFS            NFSOption    `json:"nfs"`
	Consul         ConsulConfig `json:"consul"`
	Registry       Registry     `json:"registry"`
	SSHDeliver     SSHDeliver   `json:"ssh_deliver"`
}

type NFSOption struct {
	Addr         string `json:"nfs_ip"`
	Dir          string `json:"nfs_dir"`
	MountDir     string `json:"nfs_mount_dir"`
	MountOptions string `json:"nfs_mount_opts"`
}

type SSHDeliver struct {
	SourceDir       string `json:"source_dir"`
	CACertName      string `json:"ca_crt_name"`
	Destination     string `json:"destination_dir"` // must be exist
	InitScriptName  string `json:"init_script_name"`
	CleanScriptName string `json:"clean_script_name"`
}

type ConsulConfig struct {
	ConsulIPs        string `json:"consul_ip"`
	ConsulPort       int    `json:"consul_port"`
	ConsulDatacenter string `json:"consul_dc"`
	ConsulToken      string `json:"consul_token"`
	ConsulWaitTime   int    `json:"consul_wait_time"`
}

type Registry struct {
	OsUsername string `json:"registry_os_username"`
	OsPassword string `json:"registry_os_password"`
	Domain     string `json:"registry_domain"`
	Address    string `json:"registry_ip"`
	Port       int    `json:"registry_port"`
	Username   string `json:"registry_username"`
	Password   string `json:"registry_password"`
	Email      string `json:"registry_email"`
	Token      string `json:"registry_token"`
	CACert     string `json:"registry_ca_crt"`
}
