package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
)

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

type VgInfo struct {
	VgName string `json:"VgName"`
	VgSize int    `json:"VgSize"`
	VgFree int    `json:"VgFree"`
}

type VgListRes struct {
	Err string   `json:"Err"`
	Vgs []VgInfo `json:"Vgs"`
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
	Mode      string `json:"mode"`
}

//get vglist
func GetVgList(addr string) ([]VgInfo, error) {
	uri := "http://" + addr + "/san/vglist"
	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected Response StatusCode:%d", resp.StatusCode)
	}
	res := &VgListRes{}
	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return nil, fmt.Errorf("Parse Response Body Error:%s ", err.Error())
	}

	if len(res.Err) > 0 {
		return nil, fmt.Errorf("%s", res.Err)
	}

	return res.Vgs, nil
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
func FileCopyToVolome(addr string, opt VolumeFileConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return CommonRes{Err: err.Error()}
	}

	uri := "http://" + addr + "/volume/file/cp"
	return HttpPost(uri, body)
}
