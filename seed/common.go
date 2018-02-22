package seed

import (
	"fmt"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

func checkMount(name string) bool {
	script := fmt.Sprintf("df -h %s", name)
	out, err := execCommand(script)
	if err != nil {
		logrus.Warnf("%+v", err)

		return false
	}

	return strings.Contains(out, name)
}

func mount(src, mountpoint string) error {
	script := fmt.Sprintf("mount  %s %s", src, mountpoint)

	fi, err := os.Lstat(mountpoint)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return errors.WithStack(err)
		}
	} else if err != nil {
		return errors.WithStack(err)
	}

	if fi != nil && !fi.IsDir() {
		return errors.New("already exist and it's not a directory")
	}

	_, err = execCommand(script)

	return err
}

func unmount(target string) error {
	script := fmt.Sprintf("umount  %s", target)
	_, err := execCommand(script)

	return err
}

func checkLvsVolume(vgname, name string) bool {
	script := fmt.Sprintf("lvs  %s | awk '{print $1}' ", vgname)
	out, err := execCommand(script)
	if err != nil {
		logrus.Warnf("%+v", err)

		return false
	}

	return strings.Contains(out, name)
}

func checkLvsByName(name string) bool {
	script := fmt.Sprintf("lvs | awk '{print $1}'")
	out, err := execCommand(script)
	if err != nil {
		logrus.Warnf("%+v", err)

		return false
	}

	return strings.Contains(out, name)
}

func checkVg(vgname string) bool {
	script := fmt.Sprintf("vgs | awk '{print $1}'")
	out, err := execCommand(script)
	if err != nil {
		logrus.Warnf("%+v", err)

		return false
	}

	return strings.Contains(out, vgname)
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
	out, err := execCommand(script)
	if err != nil {
		return "", err
	}

	if len(out) == 0 {
		return "", errors.Errorf("%s,get vgname by lvsnaem fail: null", script)
	}

	datastr := strings.Replace(out, "\n", "", -1)

	return datastr, nil
}

func getComonVolumePath(vgname, lvsname string) (string, error) {

	return fmt.Sprintf("/dev/%s/%s", vgname, lvsname), nil
}

func getMountPoint(vname string) string {
	return "/" + vname
}

func isDIR(path string) bool {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false
	}

	if fi != nil && !fi.IsDir() {
		return false
	}

	return true
}
