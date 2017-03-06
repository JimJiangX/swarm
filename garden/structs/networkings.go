package structs

type PostNetworkingRequest struct {
	Prefix     int    `json:"prefix"`
	VLAN       byte   `json:"vlan"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Gateway    string `json:"gateway"`
	Networking string `json:"networking_id"`
}
