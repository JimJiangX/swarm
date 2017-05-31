package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	//	"log"
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"
)

//VolumeFileCfg contains file infomation and volume placed
// used in CopyFileToVolume
type VolumeFileCfg struct {
	VgName    string `json:"VgName"`
	LvsName   string `json:"LvsName"`
	MountName string `json:"MountName"` //MountName 与 LvsName 一样
	Data      string `json:"Data"`
	FDes      string `json:"FDes"` //卷目录的相对路径
	Mode      string `json:"Mode"`
}

func volumeFileCpHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &VolumeFileCfg{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkVolumeFileCfg(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	mountpoint := getMountPoint(opt.MountName)
	if !checkMount(mountpoint) {
		src, err := getComonVolumePath(opt.VgName, opt.LvsName)
		if err != nil {
			errCommonHanlde(w, req, err)
			return
		}

		log.Println("try to mount for VolumeFileCp")
		if err := mount(src, mountpoint); err != nil {
			errCommonHanlde(w, req, err)
			return
		}

		defer func() {
			log.Info("try umount %s after copy complete", mountpoint)
			if err := unmount(mountpoint); err != nil {
				log.Errorf("umount fail:%s", err.Error())
			}
		}()
	}

	filedes := mountpoint + "/" + opt.FDes
	tempfile := mountpoint + "/" + opt.FDes + ".tmp"
	if err := doFileRepalce(opt.Data, opt.Mode, filedes, tempfile); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	log.WithFields(log.Fields{
		"data_len": len(opt.Data),
		"Mode":     opt.Mode,
		"filedes":  filedes,
	}).Info("VolumeFileCp ok")

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)
}

func checkVolumeFileCfg(opt *VolumeFileCfg) error {
	if len(opt.Data) == 0 {
		return errors.New("the data is null")
	}

	if opt.LvsName == "" {
		return errors.New("the LvsName  is null")
	}

	if opt.VgName == "" {
		return errors.New("the VgName  is null")
	}
	if !checkVg(opt.VgName) {
		return errors.New("don't find the VG")
	}

	if !checkLvsVolume(opt.VgName, opt.LvsName) {
		return errors.New("don't find the lvsName")
	}

	if opt.MountName == "" {
		return errors.New("the MountName  is null")
	}

	if opt.FDes == "" {
		return errors.New("the FDes  is null")
	}

	if opt.Mode == "" {
		return errors.New("the Mode  is null")
	}

	return nil
}

func writeToTmpfile(data, tempfile string) error {
	fi, err := os.OpenFile(tempfile, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return errors.New("try open tempfile fail: " + err.Error())
	}

	defer fi.Close()

	num, err := fi.WriteString(data)
	if err != nil {
		return errors.New("write fail: " + err.Error())
	}

	sendlen := len(data)
	if num != sendlen {
		return fmt.Errorf("data len :%d ;just write to file: %d ", sendlen, num)

	}
	return nil
}

func doFileRepalce(data, mode, filedes, tempfile string) error {
	if isDIR(filedes) {
		return errors.New("the des should not be dir")
	}

	if err := writeToTmpfile(data, tempfile); err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
		}).Error("repalce fail : WriteToTmpfile fail")
		return errors.New("write to tmp file err:" + err.Error())
	}

	mvcript := fmt.Sprintf("mv -f  %s %s", tempfile, filedes)
	_, err := execCommand(mvcript)
	if err != nil {
		log.WithFields(log.Fields{
			"err":    err.Error(),
			"script": mvcript,
		}).Error("repalce fail : mv fail:")
		return errors.New("repalce fail : mv fail:" + err.Error())
	}

	chmodscript := fmt.Sprintf("chmod  %s %s", mode, filedes)
	_, err = execCommand(chmodscript)
	if err != nil {
		log.WithFields(log.Fields{
			"err":    err.Error(),
			"script": chmodscript,
		}).Error("repalce fail : chmod fail:")
		return errors.New("repalce fail : chmod fail:" + err.Error())
	}

	return nil
}
