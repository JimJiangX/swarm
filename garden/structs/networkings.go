package structs

type PostNetworkingRequest struct {
	Prefix     int
	VLAN       byte
	Start      string
	End        string
	Gatewary   string
	Networking string `json:"networking_id"`
}

type PutNetworkingRequest struct {
	Filters []string
}
