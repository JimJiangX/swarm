package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSpace(t *testing.T) {
	src0 := `
			1 10200400 2568900 online 19
            2 107800 25600 online 2
			3 102100 256800 online 33
            4 2400 500 online 100`
	src1 := `1-1 921600 258048 NML 36
1-2 921600 258048 NML 36
1-3 921600 258048 NML 36
  1-4   921600  258048  NML  36
`
	buffer := strings.NewReader(src0)
	spaces, warnings := parseSpace(buffer)

	if len(warnings) > 0 {
		t.Error(warnings)
	}

	if len(spaces) != 4 {
		t.Error("Unexpected,", spaces)
	} else {
		t.Log(spaces)
	}

	buffer = strings.NewReader(src1)
	spaces, warnings = parseSpace(buffer)

	if len(warnings) > 0 {
		t.Error(warnings)
	}

	if len(spaces) != 4 {
		t.Error("Unexpected,", spaces)
	} else {
		t.Log(spaces)
	}
}

func TestScriptPath(t *testing.T) {
	files := []string{"connect_test.sh", "create_lun.sh", "del_lun.sh", "listrg.sh",
		"add_host.sh", "del_host.sh", "create_lunmap.sh", "del_lunmap.sh"}

	hw := huaweiStore{
		script: filepath.Join(getScriptPath(), HUAWEI),
	}

	for i := range files {
		path, err := hw.scriptPath(files[i])
		if err != nil {
			t.Error(files[i], err)
		} else {
			t.Log(files[i], path)
		}
	}

	hs := hitachiStore{
		script: filepath.Join(getScriptPath(), HITACHI, "G600"),
	}

	for i := range files {
		path, err := hs.scriptPath(files[i])
		if err != nil {
			t.Error(files[i], err)
		} else {
			t.Log(files[i], path)
		}
	}
}

func TestFindIdleNum(t *testing.T) {
	ok, got := findIdleNum(5, 10, []int{6, 9, 1, 0, 5, 88, 3, 4, 2, 40})
	if !ok || got != 7 {
		t.Error(ok, got)
	}
}
