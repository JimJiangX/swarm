package seed

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

// VolumeUpdateOpt used in VolumeUpdate
type VolumeUpdateOpt struct {
	VgName string `json:"VgName"`
	LvName string `json:"LvName"`
	FsType string `json:"FsType"`
	Size   int    `json:"Size"`
}

func updateHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
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
	writeJSON(w, res, http.StatusOK)
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

	if !checkVg(opt.VgName) {
		return errors.New("don't find the VG")
	}

	if !checkLvsVolume(opt.VgName, opt.LvName) {
		return errors.New("don't find the lvName")
	}

	return nil

}

func updateCapacity(vgname string, lvname string, size int, fstype string) (err error) {
	// lvextend /dev/VG_HDD/BFJiD51R_XX_zwLjX_DAT -L 10G
	updatescript := fmt.Sprintf("lvextend -f  /dev/%s/%s -L %dB", vgname, lvname, size)

	out, err := execCommand(updatescript)
	if err != nil {
		return err
	}

	mountpoint := "/" + lvname
	if !checkMount(mountpoint) {
		src := fmt.Sprintf("/dev/%s/%s", vgname, lvname)

		if err = mount(src, mountpoint); err != nil {
			return errors.Errorf("lvextend success but xfs_growfs fail: try mount fail:%+v", err)
		}

		defer func() {
			if _err := unmount(mountpoint); _err != nil {
				err = fmt.Errorf("%+v\n%+v", _err, err)

				logrus.Printf("try umount %s after  xfs_growfs,%+v", mountpoint, err)
			}
		}()
	}

	growfsscript := fmt.Sprintf("xfs_growfs  /dev/%s/%s ", vgname, lvname)
	out, err = execCommand(growfsscript)
	if err != nil {
		//this problem TODO later.
		return errors.Errorf("lvextend success but xfs_growfs,%s fail:%s,%s", growfsscript, out, err)
	}

	return nil
}
