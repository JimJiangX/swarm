package seed

import (
	"encoding/json"
	"errors"
	"fmt"

	"net/http"
	"os"

	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

var drivers map[string]string

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

func VgExtend(ctx *_Context, w http.ResponseWriter, req *http.Request) {
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

	if err := scanSanDisk(); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if len(opt.HostLunId) != 1 {
		errCommonHanlde(w, req, errors.New("now just support extend one device"))
		return
	}

	device, err := getDevicePath(opt.HostLunId[0], opt.Type)
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

func VgCreate(ctx *_Context, w http.ResponseWriter, req *http.Request) {
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

	if err := scanSanDisk(); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	devices := ""
	for _, id := range opt.HostLunId {
		path, err := getDevicePath(id, opt.Type)
		if err != nil {
			errCommonHanlde(w, req, err)
			return
		}
		devices = devices + "  " + path
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

func VgList(ctx *_Context, w http.ResponseWriter, req *http.Request) {

	vgs, err := vgList()

	if err != nil {
		res := &VgListRes{
			Err: err.Error(),
		}
		response, _ := json.Marshal(res)
		w.Write(response)
		return
	}

	res := &VgListRes{
		Err: "",
		Vgs: vgs,
	}
	log.Println("VGLIST:", res)
	response, _ := json.Marshal(res)
	w.Write(response)
}

func checkVgConfig(cfg *VgConfig) error {
	if len(cfg.HostLunId) == 0 {
		return errors.New("device HostLunId must not be null")
	}

	if _, ok := getDriver(cfg.Type); !ok {
		return errors.New("not support the driver now")
	}

	return nil
}

func vgList() ([]VgInfo, error) {
	vgs := []VgInfo{}
	vgscript := fmt.Sprintf("vgs --units b | sed '1d' | awk '{print $1,$6,$7}' ")

	out, err := ExecCommand(vgscript)
	log.Printf("%s\n%s\n%v\n", vgscript, string(out), err)
	if err != nil {
		errstr := "vglist exeec fail:" + err.Error()
		log.Println(errstr)
		return nil, errors.New(errstr)
	}
	outstr := string(out)
	lines := strings.Split(outstr, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		slices := strings.Fields(line)
		if len(slices) != 3 {
			log.Println("[warn]the line not 3 slice:", line)
			continue
		}

		name := slices[0]
		sizeslice := strings.Split(slices[1], ".")
		size, err := strconv.Atoi(strings.Split(sizeslice[0], "B")[0])
		if err != nil {
			log.Printf("[warn]the line %s get the VSize fail: %d", line, size)
			continue
		}

		freeslice := strings.Split(slices[2], ".")
		free, err := strconv.Atoi(strings.Split(freeslice[0], "B")[0])
		if err != nil {
			log.Printf("[warn]the line %s get the VFree fail: %d", line, free)
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

func scanSanDisk() error {
	scriptpath := SCRIPT_DIR + "sanscandisk.sh"
	_, err := os.Lstat(scriptpath)
	if os.IsNotExist(err) {
		return errors.New("not find the file:" + scriptpath)
	}

	out, err := ExecShellFile(scriptpath)

	log.Printf("%s\n%s\n%v\n", scriptpath, string(out), err)

	if err != nil {
		errstr := "scanSanDisk fail:" + err.Error()
		log.Println(errstr)
		return errors.New(errstr)
	}

	time.Sleep(3 * time.Second)
	return nil
}

func getDevicePath(id int, santype string) (string, error) {
	scriptpath := SCRIPT_DIR + "getsandevice.sh"
	_, err := os.Lstat(scriptpath)
	if os.IsNotExist(err) {
		return "", errors.New("not find the file:" + scriptpath)
	}
	args := []string{santype, strconv.Itoa(id)}

	out, err := ExecShellFile(scriptpath, args...)
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

func vgCreate(name, devices string) error {
	vgcreatesctript := fmt.Sprintf("vgcreate %s  %s ", name, devices)
	_, err := ExecCommand(vgcreatesctript)

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
	_, err := ExecCommand(extendsctript)

	if err != nil {
		log.WithFields(log.Fields{
			"ctript": extendsctript,
			"err":    err.Error(),
		}).Error("vgExtend fail")
		return err
	}

	return nil
}
