package seed

import (
	"encoding/json"
	"errors"
	"strconv"

	//	"log"
	"net"
	"net/http"
	//	log "github.com/Sirupsen/logrus"
)

type NetworkCfg struct {
	Instance string `json:"instance"`

	HDevice string `json:"hostDevice"`

	CDevice string `json:"containedDevice"`

	IpCIDR  string `json:"IpCIDR"`
	Gateway string `json:"gateway"`

	VlanId int `json:"vlanId"`

	BandWidth int `json:"bandWidth"`
}

func networkCreate(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &NetworkCfg{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := valicateNetworkCfg(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := createNetwork(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)

}

func valicateNetworkCfg(cfg *NetworkCfg) error {
	if cfg.Gateway == "" || cfg.Instance == "" || cfg.CDevice == "" {
		return errors.New("bad NetworkCfg req")
	}

	if _, _, err := net.ParseCIDR(cfg.IpCIDR); err != nil {
		return errors.New("bad NetworkCfg req(IpCIDR)")
	}

	return nil
}

func createNetwork(cfg *NetworkCfg) error {
	if cfg.CDevice == "" {
		cfg.CDevice = "eth0"
	}

	args := []string{
		"-h", cfg.HDevice,
		"-i", cfg.CDevice,
		"-c", cfg.Instance,
		"-ip", cfg.IpCIDR + "@" + cfg.Gateway,
		"-v", strconv.Itoa(cfg.VlanId),
		"-d", strconv.Itoa(cfg.BandWidth),
	}

	filepath := NET_SCRIPT_DIR + "init_nic.sh"

	_, err := ExecShellFile(filepath, args...)
	return err
}
