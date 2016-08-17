package store

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
