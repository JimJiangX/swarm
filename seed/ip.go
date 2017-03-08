package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	//	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"

	log "github.com/Sirupsen/logrus"
)

type IPDevCfg struct {
	Device string `json:"Device"`
	IpCIDR string `json:"IpCIDR"`
}

func IpRemove(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &IPDevCfg{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkIPDevCfg(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := ipRemove(opt.IpCIDR, opt.Device); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)
	log.Printf("delete  IP sucucess: %s:%s", opt.IpCIDR, opt.Device)
}

func IPCreate(ctx *_Context, w http.ResponseWriter, req *http.Request) {
	opt := &IPDevCfg{}
	dec := json.NewDecoder(req.Body)

	if err := dec.Decode(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := checkIPDevCfg(opt); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	if err := ipAppend(opt.IpCIDR, opt.Device); err != nil {
		errCommonHanlde(w, req, err)
		return
	}

	res := &CommonRes{
		Err: "",
	}
	response, _ := json.Marshal(res)
	w.Write(response)
	log.Printf("add  IP sucucess: %s:%s", opt.IpCIDR, opt.Device)
}

func checkIPDevCfg(cfg *IPDevCfg) error {
	if cfg.Device == "" {
		return errors.New("dev must not be null")
	}
	if _, _, err := net.ParseCIDR(cfg.IpCIDR); err != nil {
		return err
	}

	script := fmt.Sprintf("ifconfig %s", cfg.Device)
	command := exec.Command("/bin/bash", "-c", script)
	if err := command.Run(); err != nil {
		return errors.New("don't find the dev")
	}

	return nil
}

func ipAppend(ip, dev string) error {

	if findIp(ip, dev) {
		return nil
	}
	script := fmt.Sprintf("ip addr add %s dev %s", ip, dev)
	log.Println(script)
	command := exec.Command("/bin/bash", "-c", script)
	if err := command.Run(); err != nil {
		log.Printf("ip addr add fail :%s", err.Error())
		return fmt.Errorf("ip addr add fail :%s", err.Error())
	}

	return nil
}

func ipRemove(ip, dev string) error {
	if findIp(ip, dev) {
		script := fmt.Sprintf("ip addr del %s dev %s", ip, dev)
		log.Println(script)
		command := exec.Command("/bin/bash", "-c", script)
		if err := command.Run(); err != nil {
			log.Printf("ip addr del fail :%s", err.Error())
			return fmt.Errorf("ip addr del fail :%s", err.Error())
		}
	}

	return nil
}

func findIp(ip, dev string) bool {

	script := fmt.Sprintf("ip addr show %s", dev)
	command := exec.Command("/bin/bash", "-c", script)
	out, err := command.Output()

	log.Printf("%s\n%s\n%v\n", script, string(out), err)
	if err != nil {
		return false
	}
	return strings.Contains(string(out), ip)
}
