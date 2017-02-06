package structs

type VolumeRequire struct {
	From    string
	Name    string
	Type    string
	Driver  string
	Size    int64
	Options map[string]interface{}
}
