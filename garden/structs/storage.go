package structs

type PostSANStoreRequest struct {
	Vendor       string `json:"vendor"`
	Version      string `json:"version"`
	Addr         string `json:"ip_addr,omitempty"`
	Username     string `json:",omitempty"`
	Password     string `json:",omitempty"`
	Admin        string `json:"admin_unit,omitempty"`
	LunStart     int    `json:"lun_start,omitempty"`
	LunEnd       int    `json:"lun_end,omitempty"`
	HostLunStart int    `json:"host_lun_start,omitempty"`
	HostLunEnd   int    `json:"host_lun_end,omitempty"`
}

type SANStorageResponse struct {
	ID     string  `json:"id"`
	Vendor string  `json:"vendor"`
	Driver string  `json:"driver"`
	Total  int64   `json:"total"`
	Free   int64   `json:"free"`
	Used   int64   `json:"used"`
	Spaces []Space `json:"spaces"`
}

type Space struct {
	Enable bool   `json:"enable"`
	ID     string `json:"id"`
	Total  int64  `json:"total"`
	Free   int64  `json:"free"`
	LunNum int    `json:"lun_num"`
	State  string `json:"state"`
}
