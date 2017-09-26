package seed

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
)

//NetworkCfg used by  /network/create,which creating docker network
type NetworkCfg struct {
	ContainerID string `json:"containerID"`

	HostDevice string `json:"hostDevice"`

	ContainerDevice string `json:"containerDevice"`

	IPCIDR  string `json:"IpCIDR"`
	Gateway string `json:"gateway"`

	VlanID int `json:"vlanID"`

	BandWidth int `json:"bandWidth"`
}

func networkCreateHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &NetworkCfg{}

	if err := json.NewDecoder(req.Body).Decode(opt); err != nil {
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

	writeJSON(w, CommonRes{}, http.StatusOK)
}

func valicateNetworkCfg(cfg *NetworkCfg) error {
	if cfg.Gateway == "" || cfg.ContainerID == "" || cfg.HostDevice == "" {
		return errors.New("bad NetworkCfg req")
	}

	if _, _, err := net.ParseCIDR(cfg.IPCIDR); err != nil {
		return errors.New("bad NetworkCfg req(IPCIDR)")
	}

	return nil
}

func createNetwork(cfg *NetworkCfg) error {
	if cfg.ContainerDevice == "" {
		cfg.ContainerDevice = "eth0"
	}

	args := []string{
		"-h", cfg.HostDevice,
		"-i", cfg.ContainerDevice,
		"-c", cfg.ContainerID,
		"-ip", cfg.IPCIDR + "@" + cfg.Gateway,
		"-v", strconv.Itoa(cfg.VlanID),
		"-b", strconv.Itoa(cfg.BandWidth),
	}

	file := filepath.Join(scriptDir, "net/", "init_nic.sh")

	_, err := execShellFile(file, args...)

	return err
}

func networkUpdateHandle(ctx *_Context, w http.ResponseWriter, r *http.Request) {
	opt := &NetworkCfg{}

	if err := json.NewDecoder(r.Body).Decode(opt); err != nil {
		errCommonHanlde(w, r, err)
		return
	}

	if err := updateNetwork(opt); err != nil {
		errCommonHanlde(w, r, err)
		return
	}

	writeJSON(w, CommonRes{}, http.StatusOK)
}

// update_nic_bw.sh -h 设备名称 -b 升级后的带宽值
func updateNetwork(cfg *NetworkCfg) error {
	if cfg.HostDevice == "" {
		return errors.New("HostDevice is required")
	}

	args := []string{
		"-h", cfg.HostDevice,
		"-b", strconv.Itoa(cfg.BandWidth),
	}

	file := filepath.Join(scriptDir, "net/", "update_nic_bw.sh")

	_, err := execShellFile(file, args...)

	return err
}
