package structs

type PostImageRequest struct {
	Name   string
	Url    string
	Labels map[string]string

	Addr     string
	SSHPort  int `json:"ssh_port"`
	Username string
	Password string

	ImageConfig
}

type ImageConfig struct {
	Path    string
	Content map[string]interface{}
	KeySet  map[string]bool
	Ports   []Port
}

type Port struct {
	Port  int    `db:"port"` // auto increment
	Name  string `db:"name"`
	Proto string `db:"proto"` // tcp/udp
}
