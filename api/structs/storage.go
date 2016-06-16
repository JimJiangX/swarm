package structs

type PostSANStoreRequest struct {
	Vendor       string
	Addr         string `json:",omitempty"`
	Username     string `json:",omitempty"`
	Password     string `json:",omitempty"`
	Admin        string `json:",omitempty"`
	LunStart     int    `json:"lun_start,omitempty"`
	LunEnd       int    `json:"lun_end,omitempty"`
	HostLunStart int    `json:"host_lun_start,omitempty"`
	HostLunEnd   int    `json:"host_lun_end,omitempty"`
}

type PostRaidGroupRequest struct {
	ID int
}
