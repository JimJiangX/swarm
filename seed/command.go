package seed

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/swarm/garden/utils"
	"github.com/pkg/errors"
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
	script := make([]string, len(args)+1)
	script[0] = shell
	copy(script[1:], args)

	out, err := utils.ExecContextTimeout(nil, timeout, script...)

	logrus.Debugf("exec:%s,%s %v", script, out, err)

	if err != nil {
		return string(out), errors.Errorf("exec:%s,%s", script, err)
	}

	return string(out), nil
}

/*
func execWithTimeout(_Type execType, shell string, timeout time.Duration, args ...string) (string, error) {
	var cmd *exec.Cmd
	if _Type == commandType {
		logrus.Printf("command:%s", shell)
		cmd = exec.Command("/bin/bash", "-c", shell)
	} else {
		logrus.Printf("fpath:%s,args:%v", shell, args)
		cmd = exec.Command(shell, args...)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("cmd start err:%s", err)
	}

	isTimeout, err := cmdRunWithTimeout(cmd, timeout)

	//	errStr := stderr.String()

	//	if errStr != "" {
	//		for _, datastr := range strings.Split(errStr, "\n") {
	//			if strings.HasPrefix(strings.ToLower(datastr), "warning:") {
	//				logrus.WithFields(logrus.Fields{
	//					"cmd":  cmd,
	//					"warn": datastr,
	//				}).Debug("get warning info")
	//			} else if datastr != "" {
	//				return "", errors.New("exec error:" + datastr)
	//			}
	//		}
	//	}

	if isTimeout {
		return "", errors.New("Timeout")
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.Success() {
			return stdout.String(), nil

		} else if cmd.ProcessState != nil && cmd.ProcessState.Success() {

			return stdout.String(), nil
		}

		return "", fmt.Errorf("exec error:%s", err)
	}

	// exec successfully

	return fmt.Sprintf("%s\n%s", stdout.Bytes(), stderr.Bytes()), nil
}

func cmdRunWithTimeout(cmd *exec.Cmd, timeout time.Duration) (bool, error) {
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
		//		logrus.Println("test goroute wait out")
	}()

	var err error
	select {
	case <-time.After(timeout):
		go func() {
			<-done // allow goroutine to exit
			//			logrus.Println("test goroute timeout out")
		}()

		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
				logrus.WithFields(logrus.Fields{
					"cmd": cmd,
					"err": err.Error(),
				}).Warnf(" exec timeout kill fail: syscall.Kill error")
			}
		} else {
			logrus.WithFields(logrus.Fields{
				"cmd": cmd,
				"err": err.Error(),
			}).Warnf(" exec timeout kill  process fail")
		}

		return true, err
	case err = <-done:
		return false, err
	}
}
*/
