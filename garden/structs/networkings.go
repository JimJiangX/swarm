package structs

type PostNetworkingRequest struct {
	Prefix     int    `json:"prefix"`
	VLAN       int    `json:"vlan_id"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Gateway    string `json:"gateway"`
	Networking string `json:"networking_id"`
}

type NetworkingInfo struct {
	Prefix     int    `json:"prefix"`
	VLAN       int    `json:"vlan_id"`
	Networking string `json:"networking_id"`
	Gateway    string `json:"gateway"`
	IPs        []IP   `json:"IPs"`
}

type IP struct {
	Enabled   bool   `json:"enabled"`
	IPAddr    string `json:"ip_addr"`
	UnitID    string `json:"unit_id"`
	Engine    string `json:"engine_id"`
	Bond      string `json:"net_dev"`
	Bandwidth int    `json:"bandwidth"`
}
