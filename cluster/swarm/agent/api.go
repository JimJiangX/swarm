package sdk

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

// CommonRes common http requet response body msg
type CommonRes struct {
	Err string `json:"Err"`
}

// Error implement of error
func (res CommonRes) Error() string {
	return res.Err
}

//update.go
type VolumeUpdateOption struct {
	VgName string `json:"VgName"`
	LvName string `json:"LvName"`
	FsType string `json:"FsType"`
	Size   int    `json:"Size"`
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

// VolumeFileConfig contains file infomation and volume placed
type VolumeFileConfig struct {
	VgName    string `json:"VgName"`
	LvsName   string `json:"LvsName"`
	MountName string `json:"MountName"`
	Data      string `json:"Data"`
	FDes      string `json:"FDes"`
	Mode      string `json:"mode"`
}

// GetVgList returns remote host VG list
// addr is the remote host server agent bind address
func GetVgList(addr string) ([]VgInfo, error) {
	uri := "http://" + addr + "/san/vglist"
	resp, err := http.Get(uri)
	if err != nil {
		return nil, errors.Wrap(err, "GET:"+uri)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("GET %s:response code=%d,body=%s,%v", uri, resp.StatusCode, respBody, err)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "read request POST:"+uri+" body")
	}

	var res *VgListRes
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, errors.Wrapf(err, "JSON unmarshal GET:"+uri+" body:"+string(respBody))
	}

	if len(res.Err) > 0 {
		return nil, errors.New("GET:" + uri + " error:" + res.Err)
	}

	return res.Vgs, nil
}

// CreateIP create a IP on remote host
// addr is the remote host server agent bind address
func CreateIP(addr string, opt IPDevConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": create IP")
	}

	uri := "http://" + addr + "/ip/create"

	return HttpPost(uri, body)
}

// RemoveIP remove the IP from remote host
// addr is the remote host server agent bind address
func RemoveIP(addr string, opt IPDevConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": remove IP")
	}

	uri := "http://" + addr + "/ip/remove"
	return HttpPost(uri, body)
}

// VolumeUpdate update volume optinal on remote host
// addr is the remote host server agent bind address
func VolumeUpdate(addr string, opt VolumeUpdateOption) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": volume update")
	}

	uri := "http://" + addr + "/VolumeDriver.Update"
	return HttpPost(uri, body)
}

// SanVgCreate create new VG on remote host
// addr is the remote host server agent bind address
func SanVgCreate(addr string, opt VgConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": SAN VG create")
	}

	uri := "http://" + addr + "/san/vgcreate"
	return HttpPost(uri, body)
}

// SanVgExtend extense the specified VG Size on remote host
// addr is the remote host server agent bind address
func SanVgExtend(addr string, opt VgConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": SAN VG extend")
	}

	uri := "http://" + addr + "/san/vgextend"
	return HttpPost(uri, body)
}

// SanActivate activates the specified LV remote host
// addr is the remote host server agent bind address
func SanActivate(addr string, opt ActiveConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": SAN activate")
	}

	uri := "http://" + addr + "/san/activate"
	return HttpPost(uri, body)
}

// SanDeActivate Deactivates the specified LV on remote host
// addr is the remote host server agent bind address
func SanDeActivate(addr string, opt DeactivateConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": SAN deactivate")
	}

	uri := "http://" + addr + "/san/deactivate"
	return HttpPost(uri, body)
}

// CopyFileToVolume Post file to the specified LV on remote host
// addr is the remote host server agent bind address
func CopyFileToVolume(addr string, opt VolumeFileConfig) error {
	body, err := encodeBody(&opt)
	if err != nil {
		return errors.Wrap(err, addr+": copy file to volume")
	}

	uri := "http://" + addr + "/volume/file/cp"
	return HttpPost(uri, body)
}
