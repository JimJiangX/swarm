package smlib

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/yiduoyunQ/sm/sm-svr/consts"
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
	topology, err := GetTopology(ip, port)
	if err != nil {
		return "", err
	}

	if topology == nil {
		return "", nil
	}

	serviceStatus := consts.StatusOK

	for _, dbInfo := range topology.DataNodeGroup["default"] {
		if dbInfo.Status != consts.Normal {
			serviceStatus = consts.StatusWarning
			if dbInfo.Type == consts.Master {
				serviceStatus = consts.StatusError
				break
			}
		}
	}

	return serviceStatus, nil
}

// need close res.Body in call function
func get(ip string, port int, method, arg string) (*http.Response, error) {
	uri := "http://" + ip + ":" + strconv.Itoa(port) + "/" + method
	if arg != "" {
		uri = uri + "/" + arg
	}

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req = req.WithContext(ctx)

	return http.DefaultClient.Do(req)
}
