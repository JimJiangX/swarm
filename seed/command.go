// +build darwin linux

package seed

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
)

type execType string

const (
	defaultTimeout = 5 * time.Second

	commandType   execType = "command"
	shellFileType execType = "file"
)

//execCommand exec command with defaultTimeout
func execCommand(command string) (string, error) {
	return execWithTimeout(commandType, command, defaultTimeout)
}

//execShellFile exec shell file with defaultTimeout
func execShellFile(fpath string, args ...string) (string, error) {
	return execWithTimeout(shellFileType, fpath, defaultTimeout*12, args...)
}

func execWithTimeout(_Type execType, shell string, timeout time.Duration, args ...string) (string, error) {
	var cmd *exec.Cmd
	if _Type == commandType {
		log.Printf("command:%s", shell)
		cmd = exec.Command("/bin/bash", "-c", shell)
	} else {
		log.Printf("fpath:%s,args:%v", shell, args)
		cmd = exec.Command(shell, args...)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Start()
	if err != nil {
		return "", errors.New("cmd start err:" + err.Error())
	}

	isTimeout, err := cmdRunWithTimeout(cmd, timeout)

	errStr := stderr.String()

	if errStr != "" {
		for _, datastr := range strings.Split(errStr, "\n") {
			if strings.HasPrefix(datastr, "Warning:") {
				log.WithFields(log.Fields{
					"cmd":  cmd,
					"warn": datastr,
				}).Debug("get warning info")
			} else if datastr != "" {
				return "", errors.New("exec error:" + datastr)
			}
		}

	}

	if isTimeout {
		return "", errors.New("Timeout")
	}

	if err != nil {
		return "", errors.New("exec error:" + err.Error())
	}

	// exec successfully
	data := stdout.Bytes()
	return string(data), nil
}

func cmdRunWithTimeout(cmd *exec.Cmd, timeout time.Duration) (bool, error) {
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
		//		log.Println("test goroute wait out")
	}()

	var err error
	select {
	case <-time.After(timeout):
		go func() {
			<-done // allow goroutine to exit
			//			log.Println("test goroute timeout out")
		}()

		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
				log.WithFields(log.Fields{
					"cmd": cmd,
					"err": err.Error(),
				}).Warnf(" exec timeout kill fail: syscall.Kill error")
			}
		} else {
			log.WithFields(log.Fields{
				"cmd": cmd,
				"err": err.Error(),
			}).Warnf(" exec timeout kill  process fail")
		}

		return true, err
	case err = <-done:
		return false, err
	}
}