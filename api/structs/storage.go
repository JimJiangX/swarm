package structs

type PostSANStoreRequest struct {
	Vendor       string
	Addr         string
	Username     string
	Password     string
	Admin        string
	LunStart     int
	LunEnd       int
	HostLunStart int
	HostLunEnd   int
}

type PostRaidGroupRequest struct {
	ID int
}
