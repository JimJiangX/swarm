package storage

import (
	"strings"
	"testing"
)

func TestParseSpace(t *testing.T) {
	src := `1 10200400 2568900 online 19
2 107800 25600 online 2
3 102100 256800 online 33
4 2400 500 online 100`

	buffer := strings.NewReader(src)
	spaces := parseSpace(buffer)

	if len(spaces) != 4 {
		t.Error("Unexpected,", spaces)
	} else {
		t.Log(spaces)
	}
}

func TestFindIdleNum(t *testing.T) {
	ok, n := findIdleNum(1000, 2000, nil)
	if !ok || n != 1000 {
		t.Errorf("Unexpected,%t,%d", ok, n)
	}

	ok, n = findIdleNum(1000, 2000, []int{1006, 1999, 10010, 1000, 1001, 1002, 1003, 1005})
	if !ok || n != 1004 {
		t.Errorf("Unexpected,%t,%d", ok, n)
	}

	ok, n = findIdleNum(1000, 1006, []int{1005, 1006, 1002, 1004, 1007, 1008, 1003, 1000, 1001})
	if ok || n != 0 {
		t.Errorf("Unexpected,%t,%d", ok, n)
	}

	ok, n = findIdleNum(1000, 1009, []int{1005, 1006, 1002, 1004, 1007, 1008, 1003, 1000, 1001})
	if !ok || n != 1009 {
		t.Errorf("Unexpected,%t,%d", ok, n)
	}
}
