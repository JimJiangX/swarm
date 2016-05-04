package structs

type PostNetworkingRequest struct {
	Prefix  int
	Start   string
	End     string
	Type    string
	Gateway string
}

type PostImportPortRequest struct {
	Start   int
	End     int
	Filters []int `json:"_"`
}
