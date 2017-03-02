package structs

type PostNetworkingRequest struct {
	Prefix     int
	Start      string
	End        string
	Gatewary   string
	Networking string `json:"networking_id"`
	VLAN       string
}

type PutNetworkingRequest struct {
	Filters []string
}
