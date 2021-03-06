package sdk

import (
	"testing"
	"time"
)

func TestGetVgList(t *testing.T) {

	client, err := NewClient("127.0.0.1:5685", 6*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	vgs, err := client.GetVgList()
	if err != nil {
		t.Skip(err)
	} else {
		t.Logf("vgs:%v", vgs)
	}
}
