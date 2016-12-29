package sdk

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var (
	// Port local_plugin_volume server port
	Port = "3333"
	// IP local_plugin_volume server IP
	IP = "127.0.0.1"
)

// SetAddr sets IP and Port for Request address
func SetAddr(ip, port string) {
	Port = port
	IP = ip
}

func getIPAddr() string {
	return IP + ":" + Port
}

// postHTTP post a requst,returns response error
func postHTTP(uri string, body io.Reader) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	req, err := http.NewRequest("POST", uri, body)
	if err != nil {
		return errors.Wrap(err, "POST:"+uri)
	}

	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "POST:"+uri)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("POST %s:response code=%d,body=%s,%v", uri, resp.StatusCode, respBody, err)
	}

	if err != nil {
		return errors.Wrapf(err, "read request POST:"+uri+" body")
	}

	res := commonResonse{}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return errors.Wrapf(err, "JSON unmarshal POST:"+uri+" body:"+string(respBody))
	}

	if len(res.Err) > 0 {
		return errors.Wrap(res, "POST:"+uri)
	}

	return nil
}

// encodeBody is used to encode a request body
func encodeBody(obj interface{}) (io.Reader, error) {
	buf := bytes.NewBuffer(nil)

	err := json.NewEncoder(buf).Encode(obj)
	if err != nil {
		return nil, errors.Wrap(err, "JSON encode")
	}

	return buf, nil
}
