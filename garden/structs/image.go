package structs

type PostLoadImageRequest struct {
	Name    string
	Version string
	Path    string
	Labels  map[string]string

	ImageConfig
}

type ImageConfig struct {
	ConfigMountPath string   `json:"config_mount_path"`
	ConfigFilePath  string   `json:"config_file_path"`
	KeySets         []Keyset `json:"config_keyset"`
}

type Keyset struct {
	CanSet      bool `json:"can_set"`
	MustRestart bool `json:"must_restart"`
	Key         string
	Desc        string
}

type ConfigTemplate struct {
	Name      string
	Version   string
	Image     string
	Mount     string
	Content   []byte
	Keysets   []Keyset
	Timestamp int64
}
