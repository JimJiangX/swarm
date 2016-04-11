package structs

type PostLoadImageRequest struct {
	Name    string
	Version string
	Url     string
	Labels  map[string]string

	RegistryAddr string
	Username     string
	Password     string

	ImageConfig
}

type ImageConfig struct {
	Path    string
	Content string
	KeySet  map[string]bool
	Ports   []Port
}

type Port struct {
	Port  int    `json:",omitempty"` // auto increment
	Name  string // config.Key
	Proto string `json:",omitempty"` // tcp/udp
}
