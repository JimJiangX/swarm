package smlib

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/yiduoyunQ/sm/sm-svr/structs"
)

func Ping(ip string, port int) error {
	return post(nil, ip, port, "ping", "")
}

func Lock(ip string, port int) error {
	return post(nil, ip, port, "lock", "")
}

func UnLock(ip string, port int) error {
	return post(nil, ip, port, "unlock", "")
}

func SetProxys(ip string, port int, proxyNames []string) error {
	b, err := json.Marshal(proxyNames)
	if err != nil {
		return err
	}

	return post(b, ip, port, "proxys", "")
}

func SetProxy(ip string, port int, proxyName string) error {
	return post([]byte(proxyName), ip, port, "proxy", "")
}

func InitSm(ip string, port int, mgmPost structs.MgmPost) error {
	b, err := json.Marshal(mgmPost)
	if err != nil {
		return err
	}

	return post(b, ip, port, "init", "")
}

func Isolate(ip string, port int, name string) error {
	return post(nil, ip, port, "isolate", name)
}

func Recover(ip string, port int, name string) error {
	return post(nil, ip, port, "recover", name)
}

func AddUser(ip string, port int, user structs.User) error {
	b, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return post(b, ip, port, "addUser", "")
}

func UptUser(ip string, port int, user structs.User) error {
	b, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return post(b, ip, port, "uptUser", "")
}

func DelUser(ip string, port int, user structs.User) error {
	b, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return post(b, ip, port, "delUser", "")
}

// need close res.Body in call function
func post(body []byte, ip string, port int, method, arg string) error {
	var (
		reader   io.Reader = nil
		bodyType           = "text/plain"
		uri                = "http://" + ip + ":" + strconv.Itoa(port) + "/" + method
	)

	if arg != "" {
		uri = uri + "/" + arg
	}
	if body != nil {
		reader = bytes.NewReader(body)
		bodyType = "application/json"
	}

	res, err := http.Post(uri, bodyType, reader)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		return errors.New(string(body))
	}
        
	io.CopyN(ioutil.Discard, res.Body, 1024)

	return nil
}
