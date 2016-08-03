package smlib

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/yiduoyunQ/sm/sm-svr/structs"
)

func Ping(ip string, port int) error {
	return post([]byte(""), ip, port, "ping", "")
}

func Lock(ip string, port int) error {
	return post([]byte(""), ip, port, "lock", "")
}

func UnLock(ip string, port int) error {
	return post([]byte(""), ip, port, "unlock", "")
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
	return post([]byte(""), ip, port, "isolate", name)
}

func Recover(ip string, port int, name string) error {
	return post([]byte(""), ip, port, "recover", name)
}

func AddUser(ip string, port int, user structs.User) error {
	fmt.Printf("%+v\n", user)
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
func post(b []byte, ip string, port int, method, arg string) error {
	var res *http.Response
	var err error
	reader := bytes.NewReader(b)
	if arg == "" {
		res, err = http.Post("http://"+ip+":"+strconv.Itoa(port)+"/"+method, "POST", reader)
	} else {
		res, err = http.Post("http://"+ip+":"+strconv.Itoa(port)+"/"+method+"/"+arg, "POST", reader)
	}
	if err != nil {
		return err
	}

	defer res.Body.Close()
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		return errors.New(string(body))
	}
	return nil
}