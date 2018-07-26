package structs

type PostNetworkingRequest struct {
	Prefix     int    `json:"prefix"`
	VLAN       int    `json:"vlan_id"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Gateway    string `json:"gateway"`
	Networking string `json:"networking_id"`
}

type PutNetworkingRequest struct {
	Prefix  *int    `json:"prefix,omitempty"`
	VLAN    *int    `json:"vlan_id,omitempty"`
	Start   *string `json:"start,omitempty"`
	End     *string `json:"end,omitempty"`
	Gateway *string `json:"gateway,omitempty"`
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
