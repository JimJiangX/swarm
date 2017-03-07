package kvstore

import (
	"testing"

	"github.com/hashicorp/consul/api"
)

func TestGetHorusAddr(t *testing.T) {
	c, err := MakeClient(&api.Config{
		Address: "192.168.4.131:8500",
	}, "prefix", "8500", nil)
	if err != nil {
		t.Fatal(err)
	}

	leader := c.getLeader()
	if leader == "" {
		t.Error("Unexpected")
	} else {
		t.Log("leader:", leader)
	}

	addr, err := c.GetHorusAddr()
	if err != nil {
		t.Error(err)
	}

	t.Log("horus:", addr)
}

func TestParseIPFromHealthCheck(t *testing.T) {
	output := "TCP connect 192.168.4.123:8000: Success"
	id := "HS-192.168.4.123"

	addr := parseIPFromHealthCheck(id, output)
	if addr == "" {
		t.Error("Unexpected")
	}

	t.Logf("'%s'", addr)
}

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

func TestDeregisterToHorus(t *testing.T) {

	// body := []string{"1234567890", "0987654321"}

	//	err := deregisterToHorus(true, body...)
	//	if err != nil {
	//		t.Skip(err)
	//	}
}

/*
func TestFastPing(t *testing.T) {
	type pingTest struct {
		host string
		want bool
	}

	tests := []pingTest{
		{"192.168.2.121", true},
		{"192.168.2.130", true},
		{"178.3.32.99", false},
	}

	for i := range tests {
		ok, err := fastPing(tests[i].host, 5, true)
		if err != nil {
			t.Skip(err)
		}

		if ok != tests[i].want {
			t.Skipf("ping %s,got %t,%v", tests[i].host, ok, err)
		}
	}
}
*/
