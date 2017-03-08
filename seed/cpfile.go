package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	//	"log"
	"net/http"
	"os"
	"os/exec"

	log "github.com/Sirupsen/logrus"
)

type VolumeFileCfg struct {
	VgName    string `json:"VgName"`
	LvsName   string `json:"LvsName"`
	MountName string `json:"MountName"` //MountName 与 LvsName 一样
	Data      string `json:"Data"`
	FDes      string `json:"FDes"` //卷目录的相对路径
	Mode      string `json:"Mode"`
}

func VolumeFileCp(ctx *_Context, w http.ResponseWriter, req *http.Request) {
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

	mountpoint := GetMountPoint(opt.MountName)
	if !checkMount(mountpoint) {
		src, err := GetComonVolumePath(opt.VgName, opt.LvsName)
		if err != nil {
			errCommonHanlde(w, req, err)
			return
		}

		log.Println("try to mount for VolumeFileCp")
		if err := mount(src, mountpoint); err != nil {
			errCommonHanlde(w, req, errors.New("mount fail:"+err.Error()))
			return
		}

		defer func() {
			log.Printf("try umount %s after copy complete", mountpoint)
			if err := unmount(mountpoint); err != nil {
				log.Printf("umount fail:%s", err.Error())
			}
		}()
	}

	filedes := mountpoint + "/" + opt.FDes
	tempfile := mountpoint + "/" + opt.FDes + ".tmp"
	if err := doFileRepalce(opt.Data, opt.Mode, filedes, tempfile); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

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
	if !CheckVg(opt.VgName) {
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

func WriteToTmpfile(data, tempfile string) error {
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
		return fmt.Errorf("data len :%d ;just write to file: %d ", sendlen)

	}
	return nil
}

func doFileRepalce(data, mode, filedes, tempfile string) error {
	if IsDIR(filedes) {
		return errors.New("the des is dir!!!")
	}

	if err := WriteToTmpfile(data, tempfile); err != nil {
		return errors.New("write to tmp file err:" + err.Error())
	}

	mvcript := fmt.Sprintf("mv -f  %s %s", tempfile, filedes)
	log.Println(mvcript)
	command := exec.Command("/bin/bash", "-c", mvcript)
	out, err := command.Output()

	log.Printf("%s\n%s\n%v\n", mvcript, string(out), err)
	if err != nil {
		log.Println("mv fail: ", string(out))
		return errors.New("repalce fail : mv fail:" + string(out))
	}

	chmodscript := fmt.Sprintf("chmod  %s %s", mode, filedes)
	command = exec.Command("/bin/bash", "-c", chmodscript)
	out, err = command.Output()

	log.Printf("%s\n%s\n%v\n", chmodscript, string(out), err)

	if err != nil {
		log.Println("chmod fail.", string(out))
		return errors.New("repalce fail : chmod fail." + string(out))
	}

	return nil
}
