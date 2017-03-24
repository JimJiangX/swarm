package seed

import (
	"encoding/json"
	"errors"
	"fmt"
	//	"log"
	"net"
	"net/http"

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

}

func checkIPDevCfg(cfg *IPDevCfg) error {
	if cfg.Device == "" {
		return errors.New("dev must not be null")
	}
	if _, _, err := net.ParseCIDR(cfg.IpCIDR); err != nil {
		return err
	}

	script := fmt.Sprintf("ifconfig %s", cfg.Device)
	_, err := ExecCommand(script)
	if err != nil {
		log.WithFields(log.Fields{
			"script": script,
			"err":    err.Error(),
		}).Error("checkIPDevCfg fail")
		return errors.New("don't find the dev")
	}

	return nil
}

func ipAppend(ip, dev string) error {

	if findIp(ip, dev) {
		return nil
	}
	script := fmt.Sprintf("ip addr add %s dev %s", ip, dev)
	if _, err := ExecCommand(script); err != nil {
		log.WithFields(log.Fields{
			"script": script,
			"err":    err.Error(),
		}).Error("ip addr add fail")
		return fmt.Errorf("ip addr add fail :%s", err.Error())
	}

	log.Info("add  IP sucucess: %s:%s", ip, dev)
	return nil
}

func ipRemove(ip, dev string) error {
	if findIp(ip, dev) {
		script := fmt.Sprintf("ip addr del %s dev %s", ip, dev)
		if _, err := ExecCommand(script); err != nil {
			log.WithFields(log.Fields{
				"script": script,
				"err":    err.Error(),
			}).Error("ipRemove fail")
			return fmt.Errorf("ip addr del fail :%s", err.Error())
		}
	}
	log.Info("delete  IP sucucess: %s:%s", ip, dev)
	return nil
}

func findIp(ip, dev string) bool {

	script := fmt.Sprintf("ip addr show %s", dev)
	out, err := ExecCommand(script)
	if err != nil {
		log.WithFields(log.Fields{
			"err":    err.Error(),
			"script": script,
		}).Warn("findIp fail")
		return false
	}
	return strings.Contains(out, ip)
}
