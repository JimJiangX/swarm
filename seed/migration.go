package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	//	"log"
	"net/http"
	"os"

	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
)

type ActConfig struct {
	VgName string   `json:"VgName"`
	Lvname []string `json:"Lvname"`
}

type DeactConfig struct {
	VgName    string   `json:"VgName"`
	Lvname    []string `json:"Lvname"`
	HostLunId []int    `json:"HostLunId"`
	Vendor    string   `json:"Vendor"`
}

type RmVGConfig struct {
	VgName    string `json:"VgName"`
	Vendor    string `json:"Vendor"`
	HostLunId []int  `json:"HostLunId"`
}

func Activate(ctx *_Context, w http.ResponseWriter, req *http.Request) {
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

	if err := scanSanDisk(); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := DoActivate(opt.VgName, opt.Lvname); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	log.WithFields(log.Fields{
		"vgname": opt.VgName,
		"lvname": opt.Lvname,
	}).Info("Activate ok")

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)

}

func Deactivate(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &DeactConfig{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkDeactConfig(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := DoDeactivate(opt.VgName, opt.Lvname); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := SanBlock(opt.Vendor, opt.HostLunId); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	//do this because ,export ok ,but can see the vg
	tryImport(opt.VgName)

	if CheckVg(opt.VgName) {
		errCommonHanlde(w, req, errors.New("exec ok .but the vgname exist yet"))
		return
	}

	for _, lv := range opt.Lvname {
		mountpoint := "/" + lv
		if err := os.Remove(mountpoint); err != nil {
			log.Printf("try rm  %s  dir err:%s \n ", mountpoint, err.Error())
		}
	}

	log.WithFields(log.Fields{
		"DeactConfig": opt,
	}).Info("Deactivate ok")

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)
}

func RemoveVG(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &RmVGConfig{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if isVgExist(opt.VgName) {
		cmd := fmt.Sprintf("vgremove -f %s", opt.VgName)
		_, err := ExecCommand(cmd)
		if err != nil {
			log.WithFields(log.Fields{
				"err": err.Error(),
				"cmd": cmd,
			}).Error("VgExistexit and vgremove fail")
			errCommonHanlde(w, req, err)
			return
		}
	}

	err := SanBlock(opt.Vendor, opt.HostLunId)
	if err != nil {
		errCommonHanlde(w, req, err)
		return
	}
	log.WithFields(log.Fields{
		"RmVGConfig": opt,
	}).Info("RemoveVG ok")

	res := &CommonRes{
		Err: "",
	}

	response, _ := json.Marshal(res)
	w.Write(response)
}

func isVgExist(vgname string) bool {
	cmd := fmt.Sprintf("vgs  %s", vgname)
	_, err := ExecCommand(cmd)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
			"cmd": cmd,
		}).Error("tryImport fail")
		return false
	}

	return true
}

func tryImport(vgname string) {
	cmd := fmt.Sprintf("vgimport  %s", vgname)
	_, err := ExecCommand(cmd)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
			"cmd": cmd,
		}).Error("tryImport fail")
	}
}

func checkDeactConfig(opt *DeactConfig) error {
	if len(opt.Lvname) == 0 {
		return errors.New("Lvname must  be set")
	}

	if opt.VgName == "" || opt.Vendor == "" {
		return errors.New("VgName  and vendor must  be set")
	}

	if len(opt.HostLunId) == 0 {
		return errors.New("HostLunId must  be set")
	}

	scriptpath := SCRIPT_DIR + "sanDeviceBlock.sh"
	_, err := os.Lstat(scriptpath)
	if os.IsNotExist(err) {
		return errors.New("not find the shell: " + scriptpath)
	}

	return nil
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

func DoActivate(vgname string, lvs []string) error {

	if err := vgImport(vgname); err != nil {
		errstr := fmt.Sprintf("  %s vgimport fail:%v", vgname, err)
		log.Println(errstr)
		return errors.New(errstr)
	}

	for _, lv := range lvs {
		if !checkLvsVolume(vgname, lv) {
			errstr := fmt.Sprintf("the %s not find %s ", vgname, lv)
			log.Println(errstr)
			return errors.New(errstr)
		}
	}

	if err := lvsActivate(vgname, lvs); err != nil {
		errstr := fmt.Sprintf("vgimport ok,but : lvsActivate fail: %s", err.Error())
		log.Println(errstr)
		return errors.New(errstr)
	}

	return nil

}

func DoDeactivate(vgname string, lvs []string) error {
	// for _, lv := range lvs {
	// 	if !checkLvsVolume(vgname, lv) {
	// 		errstr := fmt.Sprintf("the %s not find %s ", vgname, lv)
	// 		log.Println(errstr)
	// 		return errors.New(errstr)
	// 	}
	// }

	if err := lvsDeActivate(vgname, lvs); err != nil {
		errstr := fmt.Sprintf("lvsDeActivate fail: %s", err)
		log.Println(errstr)
		return errors.New(errstr)
	}

	if err := vgExport(vgname); err != nil {
		errstr := fmt.Sprintf("vgExport %s fail: %s", vgname, err)
		log.Println(errstr)
		return errors.New(errstr)
	}

	return nil
}

func vgImport(vg string) error {
	cmd := fmt.Sprintf("vgimport -f %s", vg)
	_, err := ExecCommand(cmd)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
			"cmd": cmd,
		}).Error("vgImport fail")
		return err
	}
	return nil
}

func vgExport(vg string) error {
	time.Sleep(2 * time.Second)
	cmd := fmt.Sprintf("vgexport  %s", vg)
	_, err := ExecCommand(cmd)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
			"cmd": cmd,
		}).Error("vgExport fail")
		return err
	}
	return nil
}

func lvsActivate(vg string, lvs []string) error {
	for _, lv := range lvs {
		if err := lvActivate(vg, lv); err != nil {
			errstr := fmt.Sprintf("the %s:%s activate fail: ", vg, lv)
			return errors.New(errstr)
		}
	}

	return nil
}
func lvActivate(vg, lv string) error {
	cmd := fmt.Sprintf("lvchange -ay /dev/%s/%s ", vg, lv)
	_, err := ExecCommand(cmd)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
			"cmd": cmd,
		}).Error("lvActivate fail")
		return err
	}

	return nil
}

func lvsDeActivate(vg string, lvs []string) error {
	for _, lv := range lvs {
		if err := lvDeActivate(vg, lv); err != nil {
			errstr := fmt.Sprintf("the %s:%s deactivate fail.  but continue ", vg, lv)
			return errors.New(errstr)
		}
	}
	return nil
}

func lvDeActivate(vg, lv string) error {
	cmd := fmt.Sprintf("lvchange -an /dev/%s/%s ", vg, lv)
	_, err := ExecCommand(cmd)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
			"cmd": cmd,
		}).Error("lvDeActivate fail")
		return err
	}

	return nil
}

func SanBlock(vendor string, ids []int) error {
	scriptpath := SCRIPT_DIR + "sanDeviceBlock.sh"
	_, err := os.Lstat(scriptpath)
	if os.IsNotExist(err) {
		return errors.New("not find the shell: " + scriptpath)
	}

	args := []string{vendor}
	for _, id := range ids {
		args = append(args, strconv.Itoa(id))
	}

	_, err = ExecShellFile(scriptpath, args...)
	if err != nil {
		log.WithFields(log.Fields{
			"args":       args,
			"scriptpath": scriptpath,
			"err":        err.Error(),
		}).Error("SanBlock fail")
		return err
	}

	return nil
}
