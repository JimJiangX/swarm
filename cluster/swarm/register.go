package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

type registerService struct {
	Endpoint      string
	CollectorName string   `json:"collectorname,omitempty"`
	User          string   `json:"user,omitempty"`
	Password      string   `json:"pwd,omitempty"`
	Type          string   `json:"type"`
	CollectorIP   string   `json:"colletorip"`   // spell error
	CollectorPort int      `json:"colletorport"` // spell error
	MetricTags    string   `json:"metrictags"`
	Network       []string `json:"network,omitempty"`
	Status        string   `json:"status"`
	Table         string   `json:"table"`
	CheckType     string   `json:"checktype"`
}

func registerToHorus(addr string, obj []registerService) error {
	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(obj); err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/v1/agent/register", addr)
	resp, err := http.Post(url, "application/json", body)
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

func deregisterToHorus(addr string, force bool, endpoints ...string) error {
	type deregisterService struct {
		Endpoint string
	}

	obj := make([]deregisterService, len(endpoints))
	for i := range obj {
		obj[i].Endpoint = endpoints[i]
	}

	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(obj); err != nil {
		return err
	}

	path := fmt.Sprintf("http://%s/v1/agent/deregister", addr)

	req, err := http.NewRequest("POST", path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if force {
		params := make(url.Values)
		params.Set("force", "true")
		req.URL.RawQuery = params.Encode()
	}

	resp, err := http.DefaultClient.Do(req)
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
