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

type GetImageResponse struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	ImageID        string              `json:"docker_image_id"`
	Labels         map[string]string   `json:"label"`
	Enabled        bool                `json:"enabled"`
	Size           int                 `json:"size"`
	UploadAt       string              `json:"upload_at"`
	TemplateConfig ImageConfigResponse `json:"template_config"`
}

type ImageConfigResponse struct {
	ID      string                  `json:"config_id"`
	Mount   string                  `json:"config_mount_path"`
	Content string                  `json:"config_content"`
	KeySet  map[string]KeysetParams `json:"config_keyset"`
}

type UpdateUnitConfigRequest struct {
	ConfigMountPath string         `json:"config_mount_path"`
	ConfigContent   string         `json:"config_file_content"`
	KeySet          []KeysetParams `json:"config_keyset"`
}
