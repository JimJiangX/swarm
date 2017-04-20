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
		{method: http.MethodGet, code: [...]int{70, 80, 90}, want: 100708090},
		{method: http.MethodPut, code: [...]int{7, 8, 9}, want: 103070809},
		{code: [...]int{7, 8, 9}, want: 109070809},
		{method: http.MethodGet, code: [...]int{770, 880, 990}, want: 100708090},
	}

	for _, c := range tests {
		ec := errCodeV1(c.method, model(c.code[0]), category(c.code[1]), c.code[2])
		if ec.code != c.want {
			t.Errorf("expect %d but got %d", c.want, ec.code)
		}
	}
}
