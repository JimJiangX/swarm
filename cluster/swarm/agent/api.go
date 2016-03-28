package sdk

//common
type CommonRes struct {
	Err string `json:"Err"`
}

func (res CommonRes) Error() string {
	return res.Err
}

//update.go
type VolumeUpdateOption struct {
	VgName string `json:"VgName"`
	LvName string `json:"LvName"`
	FsType string `json:"FsType"`
	Size   string `json:"Size"`
}

//san.go
type VgConfig struct {
	HostLunId []int  `json:"HostLunId"`
	VgName    string `json:"VgName"`
	Type      string `json:"Type"`
}

//ip.go
type IPDevConfig struct {
	Device string `json:"Device"`
	IPCIDR string `json:"IpCIDR"`
}

//migration.go
type ActiveConfig struct {
	VgName string   `json:"VgName"`
	Lvname []string `json:"Lvname"`
}

type DeactivateConfig struct {
	VgName    string   `json:"VgName"`
	Lvname    []string `json:"Lvname"`
	HostLunId []int    `json:"HostLunId"`
	Vendor    string   `json:"Vendor"`
}

//cpfile.go
type VolumeFileConfig struct {
	VgName    string `json:"VgName"`
	LvsName   string `json:"LvsName"`
	MountName string `json:"MountName"`
	Data      string `json:"Data"`
	FDes      string `json:"FDes"`
}

//ip
func CreateIP(addr string, opt IPDevConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/ip/create"

	return HttpPost(uri, body)
}

func RemoveIP(addr string, opt IPDevConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/ip/remove"
	return HttpPost(uri, body)
}

//update
func VolumeUpdate(addr string, opt VolumeUpdateOption) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/VolumeDriver.Update"
	return HttpPost(uri, body)
}

//VG
func SanVgCreate(addr string, opt VgConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/san/vgcreate"
	return HttpPost(uri, body)
}

func SanVgExtend(addr string, opt VgConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/san/vgextend"
	return HttpPost(uri, body)
}

//migrate

func SanActivate(addr string, opt ActiveConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/san/activate"
	return HttpPost(uri, body)
}

func SanDeActivate(addr string, opt DeactivateConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/san/deactivate"
	return HttpPost(uri, body)
}

//file cp
func FileCpToVolome(addr string, opt VolumeFileConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/volume/file/cp"
	return HttpPost(uri, body)
}
