package utils

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestGenerateUUID(t *testing.T) {
	uuid := make(map[string]byte)
	for length := 2; length <= 128; length *= 2 {
		t.Log(length)
		for i := 0; i < 100; i++ {
			id := GenerateUUID(length)
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

var test = []struct {
	ip   string
	code uint32
}{
	{"192.168.41.23", 3232246039},
	{"127.0.0.1", 2130706433},
	{"10.0.2.78", 167772750},
}

func TestIPToUint32(t *testing.T) {
	for i := range test {
		got := IPToUint32(test[i].ip)
		if got != test[i].code {
			t.Fatalf("IP:%s [got:%v] != [want:%d]",
				test[i].ip, got, test[i].code)
		}
	}
}

func TestUint32ToIP(t *testing.T) {
	for i := range test {
		got := Uint32ToIP(test[i].code)
		if got.String() != test[i].ip {
			t.Fatalf("%d [got:%v] != [want:%d]",
				test[i].code, got, test[i].ip)
		}
	}
}

func TestBase64Generate(t *testing.T) {
	pairs := []string{"root:root", "abcdefghg:123456789abcdefgh", "kiajfoafalfjaf:jfaoujalmfajifoajnf"}
	for i := range pairs {
		s := strings.Split(pairs[i], ":")
		username, password := s[0], s[1]
		auth := Base64Encode(username, password)
		t.Log(i, pairs[i], auth)
		name, passwd, err := Base64Decode(auth)
		if err != nil {
			t.Fatal(err)
		}
		if name != username || passwd != password {
			t.Fatal(i, pairs[i], auth, name, passwd)
		}
	}
}

func TestExecScript(t *testing.T) {
	p, err := ExecScript("echo foo bar baz")
	if err != nil {
		t.Fatal(err)
	}
	bs, err := p.Output()
	if g, e := string(bs), "foo bar baz\n"; g != e {
		t.Errorf("echo: want %q, got %q", e, g)
	}

	input := "Input string\nLine 2"
	p, err = ExecScript("cat")
	if err != nil {
		t.Fatal(err)
	}
	p.Stdin = strings.NewReader(input)
	bs, err = p.Output()
	if err != nil {
		t.Errorf("cat: %v", err)
	}
	s := string(bs)
	if s != input {
		t.Errorf("cat: want %q, got %q", input, s)
	}
}

func TestGetPrivateIP(t *testing.T) {
	address := []string{
		"localhost", "127.0.0.1",
		"127.0.0.1", "127.0.0.1",
	}
	for i, length := 0, len(address); i < length; i = i + 2 {
		ip, err := GetPrivateIP(address[i])
		if err != nil {
			t.Error(err, address[i], address[i+1])
		}
		if ip.String() != address[i+1] {
			t.Error(ip.String(), address[i+1])
		}
	}
}

func TestGetAbsolutePath(t *testing.T) {
	abs, err := GetAbsolutePath(true, "abc", "def", "ghi")
	if err != nil {
		t.Log("expected", abs, err)

		err = os.MkdirAll(abs, os.ModePerm)
		if err != nil {
			t.Fatal(abs, err)
		}

		base, err := GetAbsolutePath(true, "abc")
		if err != nil {
			t.Log("expected", base, err)
		}
		defer os.RemoveAll(base)
	}

	abs, err = GetAbsolutePath(true, "./abc", "def", "ghi")
	if err != nil {
		t.Fatal(abs, err)
	}

	abs, err = GetAbsolutePath(true, abs)
	if err != nil {
		t.Fatal(abs, err)
	}

	abs, err = GetAbsolutePath(false, abs)
	if err == nil {
		t.Fatal("Unexpected", abs)
	}

	name := filepath.Join(abs, "aaaa.txt")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(name, err)
	}
	defer file.Close()

	abs, err = GetAbsolutePath(false, name)
	if err != nil {
		t.Fatal("Unexpected", abs)
	}

	t.Log(abs)
}
func TestParseUintList(t *testing.T) {
	valids := map[string]map[int]bool{
		"":             {},
		"7":            {7: true},
		"1-6":          {1: true, 2: true, 3: true, 4: true, 5: true, 6: true},
		"0-7":          {0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true},
		"0,3-4,7,8-10": {0: true, 3: true, 4: true, 7: true, 8: true, 9: true, 10: true},
		"0-0,0,1-4":    {0: true, 1: true, 2: true, 3: true, 4: true},
		"03,1-3":       {1: true, 2: true, 3: true},
		"3,2,1":        {1: true, 2: true, 3: true},
		"0-2,3,1":      {0: true, 1: true, 2: true, 3: true},
	}
	for k, v := range valids {
		out, err := ParseUintList(k)
		if err != nil {
			t.Fatalf("Expected not to fail, got %v", err)
		}
		if !reflect.DeepEqual(out, v) {
			t.Fatalf("Expected %v, got %v", v, out)
		}
	}

	invalids := []string{
		"this",
		"1--",
		"1-10,,10",
		"10-1",
		"-1",
		"-1,0",
	}
	for _, v := range invalids {
		if out, err := ParseUintList(v); err == nil {
			t.Fatalf("Expected failure with %s but got %v", v, out)
		}
	}
}

func TestParseTime(t *testing.T) {
	now := time.Now()
	timeString := TimeToString(now)

	t1, err := ParseStringToTime(timeString)
	if err != nil {
		t.Error(err, timeString)
	}

	if t1.Location() != time.Local {
		t.Error("Unexpected,location conflict")
	}

	sub := now.Sub(t1)
	if sub >= time.Second {
		t.Error("Unexpected", now, t1, sub)
	}

	t.Log(now, timeString, t1, sub)
}
