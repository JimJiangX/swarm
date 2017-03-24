package structs

type PostNetworkingRequest struct {
	Prefix     int    `json:"prefix"`
	VLAN       int    `json:"vlan_id"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Gateway    string `json:"gateway"`
	Networking string `json:"networking_id"`
}
