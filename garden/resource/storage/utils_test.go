package storage

import (
	"os"
	"path/filepath"
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

func TestScriptPath(t *testing.T) {
	files := []string{"connect_test.sh", "create_lun.sh", "del_lun.sh", "listrg.sh",
		"add_host.sh", "del_host.sh", "create_lunmap.sh", "del_lunmap.sh"}

	gopath := os.Getenv("GOPATH")

	hw := huaweiStore{
		script: filepath.Join(gopath, "src/github.com/docker/swarm/script", HUAWEI),
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
		script: filepath.Join(gopath, "src/github.com/docker/swarm/script", HITACHI),
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
