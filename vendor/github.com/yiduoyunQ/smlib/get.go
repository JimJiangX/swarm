package smlib

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/yiduoyunQ/sm/sm-svr/structs"
)

func GetProxy(ip string, port int, name string) (*structs.ProxyInfo, error) {
	res, err := get(ip, port, "proxy", name)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := ioutil.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(b))
	}
	var proxyInfo structs.ProxyInfo
	json.Unmarshal(b, &proxyInfo)
	return &proxyInfo, nil
}

func GetProxys(ip string, port int) (map[string]*structs.ProxyInfo, error) {
	res, err := get(ip, port, "proxys", "")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := ioutil.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(b))
	}
	var proxyInfos map[string]*structs.ProxyInfo
	json.Unmarshal(b, proxyInfos)
	return proxyInfos, nil
}

func GetTopology(ip string, port int) (*structs.Topology, error) {
	res, err := get(ip, port, "topology", "")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := ioutil.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(b))
	}
	var topology structs.Topology
	json.Unmarshal(b, &topology)
	return &topology, nil
}

// need close res.Body in call function

func get(ip string, port int, method, arg string) (*http.Response, error) {
	var res *http.Response
	var err error
	if arg == "" {
		res, err = http.Get("http://" + ip + ":" + strconv.Itoa(port) + "/" + method)
	} else {
		res, err = http.Get("http://" + ip + ":" + strconv.Itoa(port) + "/" + method + "/" + arg)
	}

	if err != nil {
		return nil, err
	}
	return res, nil
}
