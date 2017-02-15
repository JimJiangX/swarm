package structs

type PostLoadImageRequest struct {
	Timeout        int
	Name           string
	Version        string
	Path           string
	ConfigFilePath string `json:"config_file_path"`
	KeysetsFile    string `json:"config_keyset"`
	Labels         map[string]string
}

type Keyset struct {
	CanSet      bool `json:"can_set"`
	MustRestart bool `json:"must_restart"`
	Key         string
	Desc        string
	Range       string
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
