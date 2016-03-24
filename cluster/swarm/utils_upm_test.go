package swarm

import (
	"regexp"
	"testing"
)

func TestGenerateUUID(t *testing.T) {
	uuid := make(map[string]byte)
	for length := 2; length <= 128; length *= 2 {
		t.Log(length)
		for i := 0; i < 100; i++ {
			id := generateUUID(length)
			if _, exist := uuid[id]; exist {
				t.Fatalf("Should get a new ID!")
			}
			// fmt.Println(id)
			if length == 64 {
				matched, err := regexp.MatchString(
					"[\\da-f]{16}[\\da-f]{8}[\\da-f]{8}[\\da-f]{8}[\\da-f]{24}", id)
				if !matched || err != nil {
					t.Fatalf("expected match %s %v %s", id, matched, err)
				}
			}
		}
	}
}

var iptest = []struct {
	ip   string
	code uint32
}{
	{"192.168.41.23", 3232246039},
	{"127.0.0.1", 2130706433},
	{"10.0.2.78", 167772750},
}

func TestIPToUint32(t *testing.T) {
	for i := range iptest {
		got := IPToUint32(iptest[i].ip)
		if got != iptest[i].code {
			t.Fatalf("IP:%s [got:%v] != [want:%d]",
				iptest[i].ip, got, iptest[i].code)
		}
	}
}

func TestUint32ToIP(t *testing.T) {
	for i := range iptest {
		got := Uint32ToIP(iptest[i].code)
		if got.String() != iptest[i].ip {
			t.Fatalf("%d [got:%v] != [want:%d]",
				iptest[i].code, got, iptest[i].ip)
		}
	}
}
