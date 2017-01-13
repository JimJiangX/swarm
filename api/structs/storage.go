package structs

type PostSANStoreRequest struct {
	Vendor       string
	Addr         string `json:"ip_addr,omitempty"`
	Username     string `json:",omitempty"`
	Password     string `json:",omitempty"`
	Admin        string `json:"admin_unit,omitempty"`
	LunStart     int    `json:"lun_start,omitempty"`
	LunEnd       int    `json:"lun_end,omitempty"`
	HostLunStart int    `json:"host_lun_start,omitempty"`
	HostLunEnd   int    `json:"host_lun_end,omitempty"`
}

type PostRaidGroupRequest struct {
	ID string
}

type SANStorageResponse struct {
	ID     string
	Vendor string
	Driver string
	Total  int
	Free   int
	Used   int
	Spaces []Space
}

type Space struct {
	Enable bool
	ID     string
	Total  int
	Free   int
	LunNum int
	State  string
}
