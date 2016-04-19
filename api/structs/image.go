package structs

type PostLoadImageRequest struct {
	Name    string
	Version string
	Path    string            `json:"path"`
	Labels  map[string]string `json:",omitempty"`

	ImageConfig
}

type ImageConfig struct {
	ConfigPath string          `json:"config_path"`
	Content    string          `json:"config_content"`
	KeySet     map[string]bool `json:"config_keyset,omitempty"`
}
