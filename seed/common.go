package seed

import (
	"errors"
	"fmt"
	//	"log"
	"os"
	"os/exec"
	"strings"

	log "github.com/Sirupsen/logrus"
)

func checkMount(name string) bool {
	script := fmt.Sprintf("df -h %s", name)

	command := exec.Command("/bin/bash", "-c", script)
	out, err := command.Output()

	log.Printf("%s\n%s\n%v\n", script, string(out), err)

	if err != nil {
		// log.Println(script, "fail")
		return false
	}

	return strings.Contains(string(out), name)
}

func mount(src, mountpoint string) error {

	mountdatascript := fmt.Sprintf("mount  %s %s", src, mountpoint)
	log.Println(mountdatascript)

	fi, err := os.Lstat(mountpoint)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if fi != nil && !fi.IsDir() {
		errinfo := fmt.Sprintf("%v already exist and it's not a directory", mountpoint)
		log.Println(errinfo)
		return errors.New(errinfo)
	}

	command := exec.Command("/bin/bash", "-c", mountdatascript)
	out, err := command.Output()

	log.Printf("%s\n%s\n%v\n", mountdatascript, string(out), err)

	if err != nil {
		log.Println("mount fail: ", string(out))
		return errors.New("mount fail :" + string(out))
	}

	return nil
}

func unmount(target string) error {
	cmd := fmt.Sprintf("umount  %s", target)
	log.Println(cmd)
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

func checkLvsVolume(vgname, name string) bool {

	script := fmt.Sprintf("lvs  %s | awk '{print $1}' ", vgname)
	command := exec.Command("/bin/bash", "-c", script)
	out, err := command.Output()

	log.Printf("%s\n%s\n%v\n", script, string(out), err)
	if err != nil {
		log.Println("exec fail: ", string(out))
		return false
	}
	return strings.Contains(string(out), name)
}

func checkLvsByName(name string) bool {

	script := fmt.Sprintf("lvs | awk '{print $1}'")
	command := exec.Command("/bin/bash", "-c", script)
	out, err := command.Output()

	log.Printf("%s\n%s\n%v\n", script, string(out), err)
	if err != nil {
		log.Println("exec fail: ", string(out))
		return false
	}
	return strings.Contains(string(out), name)
}

func CheckVg(vgname string) bool {
	script := fmt.Sprintf("vgs | awk '{print $1}'")
	command := exec.Command("/bin/bash", "-c", script)
	out, err := command.Output()
	log.Printf("%s\n%s\n%v\n", script, string(out), err)
	if err != nil {
		log.Println("get vgs fail:" + string(out))
		return false
	}
	return strings.Contains(string(out), vgname)
}

func checkLvsVolumeName(name string) bool {
	index := strings.LastIndexByte(name, '_')
	if index == -1 {
		return false
	}

	index = strings.LastIndexByte(name[:index-1], '_')
	if index == -1 {
		return false
	}

	if name[index+1:] == "DAT_LV" || name[index+1:] == "LOG_LV" || name[index+1:] == "CNF_LV" || name[index+1:] == "USERDEF_LV" {
		return true
	}

	return false
}

func getVgName(lvsname string) (string, error) {
	script := fmt.Sprintf("lvs  | grep '%s' |awk '{print $2}'", lvsname)
	command := exec.Command("/bin/bash", "-c", script)
	log.Println(script)
	out, err := command.Output()

	log.Printf("%s\n%s\n%v\n", script, string(out), err)

	if err != nil {
		return "", errors.New("exec fail: get vgname by lvsnaem fail")
	}
	if len(out) == 0 {
		return "", errors.New(" get vgname by lvsnaem fail: null")
	}
	datastr := strings.Replace(string(out), "\n", "", -1)
	return datastr, nil
}

func GetComonVolumePath(vgname, lvsname string) (string, error) {

	volumepath := fmt.Sprintf("/dev/%s/%s", vgname, lvsname)
	return volumepath, nil
}

func GetMountPoint(vname string) string {
	return "/" + vname
}

func IsDIR(path string) bool {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false
	}

	if fi != nil && !fi.IsDir() {
		return false
	}

	return true
}

//local 相关 废弃
// func checkLocalVolume(name string) bool {

// 	//TODO should add vg
// 	script := fmt.Sprintf("lvs VG_SSD VG_HDD | awk '{print $1}' ")
// 	command := exec.Command("/bin/bash", "-c", script)
// 	out, err := command.Output()
// 	if err != nil {
// 		log.Println("exec fail: ", string(out))
// 		return false
// 	}
// 	return strings.Contains(string(out), name)
// }

// var drivers map[string]string

// func init() {
// 	drivers = map[string]string{
// 		"localdisk": "localdisk",
// 		"HDS":       "HDS",
// 		"HUAWEI":    "HUAWEI",
// 	}
// }

// func getDriver(dname string) (string, bool) {
// 	driver, ok := drivers[dname]
// 	if !ok {
// 		return "", false
// 	}
// 	return driver, true
// }

// func getLocalVgName(name string) (error, string) {

// 	if !checkLvsVolumeName(name) {
// 		return errors.New("name must be end with _DAT or _LOG"), ""
// 	}

// 	script := fmt.Sprintf("vgs | awk '{print $1}'")
// 	command := exec.Command("/bin/bash", "-c", script)
// 	out, err := command.Output()
// 	if err != nil {
// 		return errors.New("vgs get VG fail"), ""
// 	}
// 	index := strings.LastIndexByte(name, '_')

// 	tailname := name[index+1:]
// 	if tailname == "LOG" {
// 		if strings.Contains(string(out), "VG_SSD") {
// 			return nil, "VG_SSD"
// 		}
// 	}

// 	if strings.Contains(string(out), "VG_HDD") {
// 		return nil, "VG_HDD"
// 	}

// 	return errors.New("vgs not find VG_HDD "), ""

// }

// func IsLocalDriver(driver string) bool {
// 	return driver == "localdisk"
// }

// func IsHuaWeiDriver(driver string) bool {
// 	return driver == "HUAWEI"
// }

// func GetLocalVolumePath(vname string) (string, error) {
// 	err, vg := getLocalVgName(vname)
// 	if err != nil {
// 		return "", err
// 	}

// 	volumepath := fmt.Sprintf("/dev/%s/%s", vg, vname)
// 	return volumepath, nil
// }

// func GetVolumePath(dtype, vgname, lvsname string) (string, error) {
// 	driver, ok := getDriver(dtype)
// 	if !ok {
// 		return "", errors.New("dont't support the driver")
// 	}

// 	if IsLocalDriver(driver) {
// 		return GetLocalVolumePath(lvsname)
// 	}
// 	return GetComonVolumePath(vgname, lvsname)

// 	// if IsHuaWeiDriver(driver) {
// 	// 	return GetComonVolumePath(vgname, lvsname)
// 	// }

// 	// return "", errors.New("not find the driver")
// }
