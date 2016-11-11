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

func GetServiceStatus(ip string, port int) (string, error) {
	res, err := get(ip, port, "serviceStatus", "")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	b, _ := ioutil.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		return "", errors.New(string(b))
	}

	return string(b), nil
}

// need close res.Body in call function
func get(ip string, port int, method, arg string) (*http.Response, error) {
	uri := "http://" + ip + ":" + strconv.Itoa(port) + "/" + method
	if arg != "" {
		uri = uri + "/" + arg
	}

	return http.Get(uri)
}
