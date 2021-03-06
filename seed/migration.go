package seed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

// ActConfig active a VG,used in SanActivate
type ActConfig struct {
	VgName string   `json:"VgName"`
	Lvname []string `json:"Lvname"`
}

// DeactConfig used in SanDeActivate
type DeactConfig struct {
	VgName    string   `json:"VgName"`
	Lvname    []string `json:"Lvname"`
	HostLunID []int    `json:"HostLunId"`
	Vendor    string   `json:"Vendor"`
}

//RmVGConfig used in removeVG
type RmVGConfig struct {
	VgName    string `json:"VgName"`
	Vendor    string `json:"Vendor"`
	HostLunID []int  `json:"HostLunId"`
}

func activateHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &ActConfig{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkActConfig(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := scanSanDisk(ctx.scriptDir); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := doActivate(opt.VgName, opt.Lvname); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)

}

func deactivateHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &DeactConfig{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkDeactConfig(ctx.scriptDir, opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := doDeactivate(opt.VgName, opt.Lvname); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := sanBlock(ctx.scriptDir, opt.Vendor, opt.HostLunID); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	//do this because ,export ok ,but can see the vg
	tryImport(opt.VgName)

	if checkVg(opt.VgName) {
		errCommonHanlde(w, req, errors.New("exec ok .but the vgname exist yet"))
		return
	}

	for _, lv := range opt.Lvname {
		mountpoint := "/" + lv
		if err := os.Remove(mountpoint); err != nil {
			logrus.Printf("try rm  %s  dir err:%+v", mountpoint, err)
		}
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)
}

func removeVGHandle(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &RmVGConfig{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if isVgExist(opt.VgName) {
		cmd := fmt.Sprintf("vgremove -f %s", opt.VgName)
		_, err := execCommand(cmd)
		if err != nil {
			errCommonHanlde(w, req, err)
			return
		}
	}

	err := sanBlock(ctx.scriptDir, opt.Vendor, opt.HostLunID)
	if err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}

	response, _ := json.Marshal(res)
	w.Write(response)
}

func isVgExist(vgname string) bool {
	cmd := fmt.Sprintf("vgs  %s", vgname)
	_, err := execCommand(cmd)
	if err != nil {
		logrus.Errorf("tryImport fail,%+v", err)

		return false
	}

	return true
}

func tryImport(vgname string) {
	cmd := fmt.Sprintf("vgimport  %s", vgname)
	_, err := execCommand(cmd)
	if err != nil {
		logrus.Errorf("tryImport fail,%+v", err)
	}
}

func checkDeactConfig(scriptDir string, opt *DeactConfig) error {
	if len(opt.Lvname) == 0 {
		return errors.New("Lvname must  be set")
	}

	if opt.VgName == "" || opt.Vendor == "" {
		return errors.New("VgName  and vendor must  be set")
	}

	if len(opt.HostLunID) == 0 {
		return errors.New("HostLunID must  be set")
	}

	script := filepath.Join(scriptDir, "san", "block_device.sh")
	_, err := os.Lstat(script)
	if os.IsNotExist(err) {
		return errors.New("not find the shell: " + script)
	}

	return errors.WithStack(err)
}

func checkActConfig(opt *ActConfig) error {
	if len(opt.Lvname) == 0 {
		return errors.New("Lvname must  be set")
	}

	if opt.VgName == "" {
		return errors.New("VgName  and vendor must  be set")
	}

	return nil
}

func doActivate(vgname string, lvs []string) error {
	if err := vgImport(vgname); err != nil {
		return err
	}

	for _, lv := range lvs {
		if !checkLvsVolume(vgname, lv) {
			return errors.Errorf("the %s not find %s ", vgname, lv)
		}
	}

	if err := lvsActivate(vgname, lvs); err != nil {
		return errors.Wrap(err, "vgimport ok,but : lvsActivate fail")
	}

	return nil

}

func doDeactivate(vgname string, lvs []string) error {
	// for _, lv := range lvs {
	// 	if !checkLvsVolume(vgname, lv) {
	// 		errstr := fmt.Sprintf("the %s not find %s ", vgname, lv)
	// 		logrus.Println(errstr)
	// 		return errors.New(errstr)
	// 	}
	// }

	if err := lvsDeActivate(vgname, lvs); err != nil {
		return err
	}

	if err := vgExport(vgname); err != nil {
		return err
	}

	return nil
}

func vgImport(vg string) error {
	cmd := fmt.Sprintf("vgimport -f %s", vg)
	_, err := execCommand(cmd)
	if err != nil {
		return err
	}

	return nil
}

func vgExport(vg string) error {
	time.Sleep(2 * time.Second)
	cmd := fmt.Sprintf("vgexport  %s", vg)

	_, err := execCommand(cmd)
	if err != nil {
		return err
	}

	return nil
}

func lvsActivate(vg string, lvs []string) error {
	for _, lv := range lvs {
		if err := lvActivate(vg, lv); err != nil {
			return err
		}
	}

	return nil
}
func lvActivate(vg, lv string) error {
	cmd := fmt.Sprintf("lvchange -ay /dev/%s/%s ", vg, lv)

	_, err := execCommand(cmd)
	if err != nil {
		return err
	}

	return nil
}

func lvsDeActivate(vg string, lvs []string) error {
	for _, lv := range lvs {
		if err := lvDeActivate(vg, lv); err != nil {
			return err
		}
	}

	return nil
}

func lvDeActivate(vg, lv string) error {
	cmd := fmt.Sprintf("lvchange -an /dev/%s/%s ", vg, lv)
	_, err := execCommand(cmd)
	if err != nil {
		return err
	}

	return nil
}

func sanBlock(scriptDir, vendor string, ids []int) error {
	script := filepath.Join(scriptDir, "san", "block_device.sh")
	_, err := os.Lstat(script)
	if os.IsNotExist(err) {
		return errors.New("not find the shell: " + script)
	}
	if err != nil {
		return errors.Wrap(err, script)
	}

	args := []string{vendor}
	for _, id := range ids {
		args = append(args, strconv.Itoa(id))
	}

	_, err = execShellFile(script, args...)

	return err
}
