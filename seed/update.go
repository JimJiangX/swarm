package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	//	"log"
	"net/http"
	"os/exec"
	// "strconv"
	log "github.com/Sirupsen/logrus"
)

type VolumeUpdateOpt struct {
	VgName string `json:"VgName"`
	LvName string `json:"LvName"`
	FsType string `json:"FsType"`
	// Size   string `json:"Size"`
	Size int `json:"Size"`
}

func Update(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &VolumeUpdateOpt{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkVolumeUpdateOpt(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := updateCapacity(opt.VgName, opt.LvName, opt.Size, opt.FsType); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)
}

func checkVolumeUpdateOpt(opt *VolumeUpdateOpt) error {

	if opt.LvName == "" {
		return errors.New("the volume  name must be set")
	}

	//just temporary
	if opt.FsType != "xfs" {
		return errors.New("now just support xfs")
	}

	// if tmp, err := strconv.Atoi(opt.Size); err != nil || tmp <= 1 {
	// 	return errors.New("the volume  size must be positive")
	// }

	if opt.Size <= 0 {
		return errors.New("the volume  size must be positive")
	}

	if opt.VgName == "" {
		return errors.New("the VgName  is null")
	}

	if !CheckVg(opt.VgName) {
		return errors.New("don't find the VG")
	}

	if !checkLvsVolume(opt.VgName, opt.LvName) {
		return errors.New("don't find the lvName")
	}

	return nil

}

func updateCapacity(vgname string, lvname string, size int, fstype string) error {
	// lvextend /dev/VG_HDD/BFJiD51R_XX_zwLjX_DAT -L 10G
	updatescript := fmt.Sprintf("lvextend -f  /dev/%s/%s -L %dB", vgname, lvname, size)
	log.Println(updatescript)

	command := exec.Command("/bin/bash", "-c", updatescript)
	out, err := command.Output()
	log.Printf("%s\n%s\n%v\n", updatescript, string(out), err)
	if err != nil {
		log.Println("lvextend fail: ")
		return errors.New("lvextend fail")
	}

	mountpoint := "/" + lvname
	src := fmt.Sprintf("/dev/%s/%s", vgname, lvname)
	if !checkMount(mountpoint) {
		log.Println("try to mount for xfs_growfs")
		if err := mount(src, mountpoint); err != nil {
			return errors.New("lvextend success but xfs_growfs fail: try mount fail:" + err.Error())
		}

		defer func() {
			log.Printf("try umount %s after  xfs_growfs", mountpoint)
			if err := unmount(mountpoint); err != nil {
				log.Printf("umount fail:%s", err.Error())
			}
		}()
	}

	growfsscript := fmt.Sprintf("xfs_growfs  /dev/%s/%s ", vgname, lvname)
	growcommand := exec.Command("/bin/bash", "-c", growfsscript)
	log.Println(growfsscript)
	out, err = growcommand.Output()
	log.Printf("%s\n%s\n%v\n", growfsscript, string(out), err)
	if err != nil {
		log.Println("lvextend success but xfs_growfs fail: ", string(out))

		//this problem TODO later.
		return errors.New("lvextend success but xfs_growfs fail: " + string(out))
	}

	return nil
}
