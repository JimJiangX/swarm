package sdk

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"time"

	httpclient "github.com/docker/swarm/plugin/client"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// commonResonse common http requet response body msg
type commonResonse struct {
	Err string `json:"Err"`
}

func (resp commonResonse) Error() string {
	return resp.Err
}

// VolumeUpdateOption used in VolumeUpdate
type VolumeUpdateOption struct {
	VgName string `json:"VgName"`
	LvName string `json:"LvName"`
	FsType string `json:"FsType"`
	Size   int    `json:"Size"`
}

// VgConfig contains VGName&Type and HostLUNID on SAN storage
// used in SanVgExtend
type VgConfig struct {
	HostLunID []int  `json:"HostLunId"`
	VgName    string `json:"VgName"`
	Type      string `json:"Type"`
}

// VgInfo contains VG total size and free size,unit:byte
// used in GetVgList response
type VgInfo struct {
	VgName string `json:"VgName"`
	VgSize int    `json:"VgSize"`
	VgFree int    `json:"VgFree"`
}

// vgListResonse response of /san/vglist
type vgListResonse struct {
	Err string   `json:"Err"`
	Vgs []VgInfo `json:"Vgs"`
}

// ActiveConfig active a VG,used in SanActivate
type ActiveConfig struct {
	VgName string   `json:"VgName"`
	Lvname []string `json:"Lvname"`
}

// DeactivateConfig used in SanDeActivate
type DeactivateConfig struct {
	VgName    string   `json:"VgName"`
	Lvname    []string `json:"Lvname"`
	HostLunID []int    `json:"HostLunId"`
	Vendor    string   `json:"Vendor"`
}

// VolumeFileConfig contains file infomation and volume placed
// used in CopyFileToVolume
type VolumeFileConfig struct {
	VgName    string `json:"VgName"`
	LvsName   string `json:"LvsName"`
	MountName string `json:"MountName"`
	Data      string `json:"Data"`
	FDes      string `json:"FDes"`
	Mode      string `json:"mode"`
}

//NetworkConfig used by  /network/create
type NetworkConfig struct {
	Container string `json:"containerID"`

	HostDevice string `json:"hostDevice"`

	ContainerDevice string `json:"containerDevice"`

	IPCIDR  string `json:"IpCIDR"`
	Gateway string `json:"gateway"`

	VlanID int `json:"vlanID"`

	BandWidth int `json:"bandWidth"`
}

type client struct {
	addr string
	c    httpclient.Client
}

//NewClient is exported
func NewClient(addr string, timeout time.Duration, tlsConfig *tls.Config) (ClientAPI, error) {

	if err := checkAddr(addr); err != nil {
		return nil, errors.Wrap(err, "checkAddr:"+addr)
	}

	cli := httpclient.NewClient(addr, timeout, tlsConfig)
	c := client{addr: addr, c: cli}

	return c, nil
}

//ClientAPI is exported
type ClientAPI interface {
	GetVgList() ([]VgInfo, error)

	VolumeUpdate(opt VolumeUpdateOption) error

	CopyFileToVolume(opt VolumeFileConfig) error

	SanDeActivate(opt DeactivateConfig) error
	SanActivate(opt ActiveConfig) error
	SanVgCreate(opt VgConfig) error
	SanVgExtend(opt VgConfig) error
	CreateNetwork(ctx context.Context, opt NetworkConfig) error
	UpdateNetwork(ctx context.Context, opt NetworkConfig) error
}

//create network for container(use pipework),which network mode is none
func (c client) CreateNetwork(ctx context.Context, opt NetworkConfig) error {
	return c.postWrap(ctx, "/network/create", opt)
}

func (c client) UpdateNetwork(ctx context.Context, opt NetworkConfig) error {
	const url = "/network/update"

	resp, err := httpclient.RequireOK(c.c.Put(ctx, url, opt))
	if err != nil {
		return err
	}

	defer httpclient.EnsureBodyClose(resp)

	res := commonResonse{}
	err = decodeBody(resp, &res)
	if err != nil {
		return errors.Errorf("%s:%s%s,%s", http.MethodPut, c.addr, url, err)
	}

	if len(res.Err) > 0 {
		return errors.Errorf("%s:%s%s,%s", http.MethodPut, c.addr, url, res.Err)
	}

	return nil
}

// GetVgList returns remote host VG list
// addr is the remote host server agent bind address
func (c client) GetVgList() ([]VgInfo, error) {

	var res vgListResonse
	const uri = "/san/vglist"

	resp, err := httpclient.RequireOK(c.c.Get(nil, uri))
	if err != nil {
		return nil, err
	}

	defer httpclient.EnsureBodyClose(resp)

	err = decodeBody(resp, &res)
	if len(res.Err) > 0 {
		return nil, errors.Errorf("%s:%s%s,%s", http.MethodGet, c.addr, uri, res.Err)
	}

	return res.Vgs, nil
}

// VolumeUpdate update volume optinal on remote host
// addr is the remote host server agent bind address
func (c client) VolumeUpdate(opt VolumeUpdateOption) error {
	return c.postWrap(nil, "/VolumeDriver.Update", opt)
}

// SanVgCreate create new VG on remote host
// addr is the remote host server agent bind address
func (c client) SanVgCreate(opt VgConfig) error {
	return c.postWrap(nil, "/san/vgcreate", opt)
}

// SanVgExtend extense the specified VG Size on remote host
// addr is the remote host server agent bind address
func (c client) SanVgExtend(opt VgConfig) error {
	return c.postWrap(nil, "/san/vgextend", opt)
}

// SanActivate activates the specified LV remote host
// addr is the remote host server agent bind address
func (c client) SanActivate(opt ActiveConfig) error {
	return c.postWrap(nil, "/san/activate", opt)
}

// SanDeActivate Deactivates the specified LV on remote host
// addr is the remote host server agent bind address
func (c client) SanDeActivate(opt DeactivateConfig) error {
	return c.postWrap(nil, "/san/deactivate", opt)
}

// CopyFileToVolume Post file to the specified LV on remote host
// addr is the remote host server agent bind address
func (c client) CopyFileToVolume(opt VolumeFileConfig) error {
	return c.postWrap(nil, "/volume/file/cp", opt)

}

// decodeBody is used to JSON decode a body
func decodeBody(resp *http.Response, out interface{}) error {
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}

func checkAddr(addr string) error {

	// validate addr is in host:port form. Use net function to handle both IPv4/IPv6 cases.
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.Wrap(err, "please validate addr is in host:port form")
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return errors.Wrap(err, "strconv.Atoi port fail")
	}

	if portNum <= 0 || portNum > 65535 {
		return errors.Wrap(err, " port should:  portNum > 0 && portNum <= 65535")
	}

	return nil
}

func (c client) postWrap(ctx context.Context, url string, opt interface{}) error {
	res := commonResonse{}

	resp, err := httpclient.RequireOK(c.c.Post(ctx, url, opt))
	if err != nil {
		return err
	}

	defer httpclient.EnsureBodyClose(resp)

	err = decodeBody(resp, &res)
	if err != nil {
		return errors.Errorf("%s:%s%s,%s", http.MethodPost, c.addr, url, err)
	}

	if len(res.Err) > 0 {
		return errors.Errorf("%s:%s%s,%s", http.MethodPost, c.addr, url, res.Err)
	}

	return nil
}
