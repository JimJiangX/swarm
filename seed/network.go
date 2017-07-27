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
