package structs

type PostLoadImageRequest struct {
	Name    string
	Version string
	Path    string
	Labels  map[string]string

	ImageConfig
}

type ImageConfig struct {
	ConfigMountPath string         `json:"config_mount_path"`
	ConfigFilePath  string         `json:"config_file_path"`
	KeySet          []KeysetParams `json:"config_keyset"`
}

type KeysetParams struct {
	Key         string
	CanSet      bool   `json:"can_set"`
	MustRestart bool   `json:"must_restart"`
	Description string `json:",omitempty"`
}
