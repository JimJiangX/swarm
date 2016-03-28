package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var (
	Port = "3333"
	IP   = "127.0.0.1"
)

func SetAddr(ip, port string) {
	Port = port
	IP = ip
}

func getIpAddr() string {
	return IP + ":" + Port
}

func HttpPost(uri string, body io.Reader) error {
	resp, err := http.Post(uri, "application/json", body)

	if err != nil {
		return CommonRes{Err: "http post err:" + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buf := bytes.NewBuffer(nil)
		io.Copy(buf, resp.Body)
		return CommonRes{Err: fmt.Sprintf("Unexpected response code:%d (%s)", resp.StatusCode, buf.String())}
	}

	res := &CommonRes{}
	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return CommonRes{Err: "Parse response body Error: " + err.Error()}
	}

	if len(res.Err) > 0 {
		return res
	}

	return nil
}

// encodeBody is used to encode a request body
func encodeBody(obj interface{}) (io.Reader, error) {
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(obj); err != nil {
		return nil, err
	}

	return buf, nil
}
