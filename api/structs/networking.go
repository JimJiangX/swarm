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

type ListNetworkingsResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Gateway string `json:"gateway"`
	Enabled bool   `json:"enabled"`
	Total   int    `json:"total"`
	Used    int    `json:"used"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type IPInfo struct {
	IPAddr       string `json:"address"`
	Prefix       int    `json:"prefix"`
	NetworkingID string `json:"networking_id"`
	UnitID       string `json:"unit_id"`
	Allocated    string `json:"allocated"`
}

type PortResponse struct {
	Port      int    `json:"port"`
	Name      string `json:"name"`
	UnitID    string `json:"unit_id"`
	UnitName  string `json:"unit_name"`
	Proto     string `json:"proto"` // tcp/udp
	Allocated bool   `json:"allocated"`
}
