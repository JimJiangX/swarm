package api

import (
	"net/http"
	"testing"
)

func TestErrCode(t *testing.T) {
	var tests = []struct {
		method string
		code   [3]int
		want   int
	}{
		{method: http.MethodGet, code: [...]int{70, 80, 90}, want: 1070180090},
		{method: http.MethodPut, code: [...]int{7, 8, 9}, want: 1007408009},
		{code: [...]int{7, 8, 9}, want: 1007008009},
		{method: http.MethodGet, code: [...]int{7770, 8880, 9990}, want: 1070180990},
	}

	for _, c := range tests {
		ec := errCodeV1(c.method, model(c.code[0]), category(c.code[1]), c.code[2])
		if ec.code != c.want {
			t.Errorf("expect %d but got %d", c.want, ec.code)
		}
	}
}
