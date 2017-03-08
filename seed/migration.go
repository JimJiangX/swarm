package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	//	"log"
	"net/http"
	"os"
	"os/exec"
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
		cmd := exec.Command("vgremove", "-f", opt.VgName)
		out, err := cmd.Output()
		log.Printf("exec:%s\n%s\n%s\n", cmd.Args, string(out), err)
		if err != nil {
			errCommonHanlde(w, req, err)
			return
		}
	}

	err := SanBlock(opt.Vendor, opt.HostLunId)
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
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	log.Printf("exec:%s\n%s\n%s\n", cmd, string(out), err)
	if err != nil {
		return false
	}
	return true
}

func tryImport(vgname string) {
	cmd := fmt.Sprintf("vgimport  %s", vgname)
	log.Println("do this because .export ok ,but can see the vg  :", cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()

	log.Printf("%s\n%s\n%v\n", cmd, string(out), err)
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
		errstr := fmt.Sprintf("  %s vgimport fail:  ", vgname, err.Error())
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
		errstr := fmt.Sprintf("lvsDeActivate fail: %s", err.Error())
		log.Println(errstr)
		return errors.New(errstr)
	}

	if err := vgExport(vgname); err != nil {
		errstr := fmt.Sprintf("vgExport %s fail:  ", vgname, err.Error())
		log.Println(errstr)
		return errors.New(errstr)
	}

	return nil
}

func vgImport(vg string) error {
	cmd := fmt.Sprintf("vgimport -f %s", vg)
	log.Println(cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}
	log.Printf("%s\n%s\n%v\n", cmd, string(out), err)
	return nil
}

func vgExport(vg string) error {
	time.Sleep(2 * time.Second)
	cmd := fmt.Sprintf("vgexport  %s", vg)
	log.Println(cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}

	log.Printf("%s\n%s\n%v\n", cmd, string(out), err)

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
	log.Println(cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}

	log.Printf("%s\n%s\n%v\n", cmd, string(out), err)

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
	log.Println(cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}
	log.Printf("%s\n%s\n%v\n", cmd, string(out), err)

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

	log.Println(scriptpath, "args:", args)

	command := exec.Command(scriptpath, args...)
	out, err := command.Output()
	log.Printf("%s\n%s\n%v\n", scriptpath, out, err)
	if err != nil {
		errstr := "sanDeviceBlock fail ." + string(out)
		log.Println(errstr)
		return errors.New(errstr)
	}

	return nil
}
