package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

/*
// node
[
{
  "endpoint": "00a4ed42828a484a76d902cee9f3396426a495a75366ced6ed9a25fc6d8c1c82",
  "collectorname": "",
  "user": "",
  "pwd": "",
  "type": "host",
  "colletorip": "192.168.16.41",
  "colletorport": 8123,
  "metrictags": "00a4ed42828a484a76d902cee9f3396426a495a75366ced6ed9a25fc6d8c1c82",
  "network": [
    "bond0",
    "bond1"
  ],
  "status": "on",
  "table": "host",
  "CheckType": "health"
}
]
// unit
[
{
  "endpoint": "PMTBTpyAtFUTHJfg17JoFLyVUS8HAur4UG4N2QQS4O1bEk2TjlrvXwVbbtXhZg4X",
  "collectorname": "PMTBTpyA_XX_kjjy8",
  "user": "mon",
  "pwd": "123.com",
  "type": "upsql",
  "colletorip": "192.168.16.41",
  "colletorport": 8123,
  "metrictags": "00a4ed42828a484a76d902cee9f3396426a495a75366ced6ed9a25fc6d8c1c82",
  "network": [],
  "status": "on",
  "table": "host",
  "CheckType": "health"
}
]
*/
const registerURL = "/v1/agent/register"

type registerService struct {
	Endpoint      string
	CollectorName string `json:",omitempty"`
	User          string `json:",omitempty"`
	Password      string `json:"pwd,omitempty"`
	Type          string
	CollectorIP   string
	CollectorPort int
	MetricTags    string
	Network       []string `json:",omitempty"`
	Status        string
	Table         string
	CheckType     string
}

func registerToHorus(addr string, obj ...registerService) error {
	buffer := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buffer).Encode(obj); err != nil {
		return err
	}

	resp, err := http.Post("http://"+addr+registerURL, "application/json", buffer)
	if err != nil {
		return err
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		res := struct {
			Err string
		}{}

		err := json.NewDecoder(resp.Body).Decode(&res)
		if err != nil {
			return err
		}
		return fmt.Errorf("StatusCode:%d,Error:%s", resp.StatusCode, res.Err)
	}

	return nil
}
