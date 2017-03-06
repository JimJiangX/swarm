package structs

type PostNetworkingRequest struct {
	Prefix     int
	VLAN       byte
	Start      string
	End        string
	Gateway    string
	Networking string `json:"networking_id"`
}
