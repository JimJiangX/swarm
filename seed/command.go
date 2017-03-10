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

type ExecType string

const (
	defaultTimeout = 5 * time.Second

	Command   ExecType = "command"
	ShellFile ExecType = "file"
)

func ExecCommand(command string) (string, error) {
	return ExecWithTimeout(Command, command, defaultTimeout)
}

func ExecShellFile(fpath string, args ...string) (string, error) {
	return ExecWithTimeout(ShellFile, fpath, defaultTimeout*12, args...)
}

func ExecWithTimeout(_Type ExecType, shell string, timeout time.Duration, args ...string) (string, error) {
	var cmd *exec.Cmd
	if _Type == Command {
		log.Debugln("command:%s", shell)
		cmd = exec.Command("/bin/bash", "-c", shell)
	} else {
		log.Debugln("fpath:%s,args:%v", shell, args)
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

	err, isTimeout := cmdRunWithTimeout(cmd, timeout)

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

func cmdRunWithTimeout(cmd *exec.Cmd, timeout time.Duration) (error, bool) {
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case <-time.After(timeout):
		go func() {
			<-done // allow goroutine to exit
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

		return err, true
	case err = <-done:
		return err, false
	}
}
