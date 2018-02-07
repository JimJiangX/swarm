package seed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

var drivers map[string]string

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

// VgListRes response of /san/vglist
type VgListRes struct {
	Err string   `json:"Err"`
	Vgs []VgInfo `json:"Vgs"`
}

func init() {
	drivers = map[string]string{
		"HITACHI": "HITACHI",
		"HUAWEI":  "HUAWEI",
	}
}

func getDriver(name string) (string, bool) {
	driver, ok := drivers[name]

	return driver, ok
}

func vgExtendHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &VgConfig{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkVgConfig(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := scanSanDisk(ctx.scriptDir); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if len(opt.HostLunID) != 1 {
		errCommonHanlde(w, req, errors.New("now just support extend one device"))
		return
	}

	device, err := getDevicePath(ctx.scriptDir, opt.Type, opt.HostLunID[0])
	if err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := vgExtend(opt.VgName, device); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)

}

func vgCreateHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &VgConfig{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkVgConfig(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := scanSanDisk(ctx.scriptDir); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	devices := ""
	for _, id := range opt.HostLunID {
		path, err := getDevicePath(ctx.scriptDir, opt.Type, id)
		if err != nil {
			errCommonHanlde(w, req, err)
			return
		}
		devices = devices + "  " + path
	}

	if err := pvCreate(devices); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := vgCreate(opt.VgName, devices); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)
}

func vgListHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	vgs, err := vgList()
	if err != nil {
		log.Errorf("%+v", err)

		writeJSON(w, VgListRes{
			Err: err.Error(),
		}, http.StatusInternalServerError)

		return
	}

	res := VgListRes{
		Err: "",
		Vgs: vgs,
	}
	log.Debug("VGLIST:", res)

	writeJSON(w, res, http.StatusOK)
}

func checkVgConfig(cfg *VgConfig) error {
	if len(cfg.HostLunID) == 0 {
		return errors.New("device HostLunID must not be null")
	}

	if _, ok := getDriver(cfg.Type); !ok {
		return errors.New("not support the driver now")
	}

	return nil
}

func vgList() ([]VgInfo, error) {
	script := fmt.Sprintf("vgs --units b | sed '1d' | awk '{print $1,$6,$7}' ")

	out, err := execCommand(script)
	log.Debugf("%s\n%s\n%v\n", script, string(out), err)
	if err != nil {
		return nil, errors.Errorf("vglist exeec fail:%s", err)
	}

	lines := strings.Split(string(out), "\n")
	vgs := make([]VgInfo, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		slices := strings.Fields(line)
		if len(slices) != 3 {
			log.Debug("[warn]the line not 3 slice:", line)
			continue
		}

		name := slices[0]
		sizeslice := strings.Split(slices[1], ".")
		size, err := strconv.Atoi(strings.Split(sizeslice[0], "B")[0])
		if err != nil {
			log.Debugf("[warn]the line %s get the VSize fail: %d", line, size)
			continue
		}

		freeslice := strings.Split(slices[2], ".")
		free, err := strconv.Atoi(strings.Split(freeslice[0], "B")[0])
		if err != nil {
			log.Debugf("[warn]the line %s get the VFree fail: %d", line, free)
			continue
		}

		vginfo := VgInfo{
			VgName: name,
			VgSize: size,
			VgFree: free,
		}

		vgs = append(vgs, vginfo)

	}

	return vgs, nil

}

func scanSanDisk(scriptDir string) error {
	script := filepath.Join(scriptDir, "san", "scan_device.sh")
	_, err := os.Lstat(script)
	if os.IsNotExist(err) {
		return errors.New("not find the file:" + script)
	}

	out, err := execShellFile(script)

	log.Printf("%s\n%s\n%v\n", script, string(out), err)

	if err != nil {
		errstr := "scanSanDisk fail:" + err.Error()
		log.Println(errstr)
		return errors.New(errstr)
	}

	time.Sleep(3 * time.Second)
	return nil
}

func getDevicePath(scriptDir, santype string, id int) (string, error) {
	script := filepath.Join(scriptDir, "san", "get_device_path.sh")
	_, err := os.Lstat(script)
	if os.IsNotExist(err) {
		return "", errors.New("not find the file:" + script)
	}
	args := []string{santype, strconv.Itoa(id)}

	out, err := execShellFile(script, args...)
	if err != nil {
		return "", err
	}

	devstr := strings.Replace(string(out), "\n", "", -1)
	if devstr == "" {
		errstr := "getDevicePath fail: device name is null."
		log.Println(errstr)
		return "", errors.New(errstr)
	}

	log.Println("getDevicePath: ", devstr)
	return devstr, nil
}

func pvCreate(devices string) error {
	pvcreatesctript := fmt.Sprintf("pvcreate -ff -y %s ", devices)
	_, err := execCommand(pvcreatesctript)

	if err != nil {
		log.WithFields(log.Fields{
			"cript": pvcreatesctript,
			"err":   err.Error(),
		}).Error("pvCreate fail")
		return err
	}

	return nil
}

func vgCreate(name, devices string) error {
	vgcreatesctript := fmt.Sprintf("vgcreate %s  %s ", name, devices)
	_, err := execCommand(vgcreatesctript)

	if err != nil {
		log.WithFields(log.Fields{
			"cript": vgcreatesctript,
			"err":   err.Error(),
		}).Error("vgCreate fail")
		return err
	}

	return nil
}

func vgExtend(name, devices string) error {
	extendsctript := fmt.Sprintf("vgextend  -f %s  %s ", name, devices)
	_, err := execCommand(extendsctript)

	if err != nil {
		log.WithFields(log.Fields{
			"ctript": extendsctript,
			"err":    err.Error(),
		}).Error("vgExtend fail")
		return err
	}

	return nil
}
