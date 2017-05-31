package seed

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestexecCommand(t *testing.T) {
	t.Skip("disable TestexecCommand")

	script := fmt.Sprintf("df -h %s", "/home")
	data, err := execCommand(script)
	assert.Nil(t, err)
	fmt.Println("data:", data)

	script = "sleep 20"
	data, err = execCommand(script)
	assert.Equal(t, err, errors.New("Timeout"))

}

func TestexecShellFile(t *testing.T) {
	//	 #!/bin/bash
	//     echo "test"
	//
	fpath := "/tmp/test.sh"
	data, err := execShellFile(fpath)
	if err != nil {
		t.Log(err)
	}
	fmt.Println("test data:", data)

	//	#!/bin/bash
	//     echo "testargs :$1,$@"
	//
	fpath = "/tmp/testargs.sh"
	data, err = execShellFile(fpath, "-d", "test1", "-f", "test2")
	if err != nil {
		t.Log(err)
	}
	fmt.Println("testargs data:", data)

	//	#!/bin/bash
	//      sleep 120
	//
	fpath = "/tmp/testtimeout.sh"
	data, err = execShellFile(fpath, "test1", "test2")
	if err != nil {
		t.Log(err)
	}
}
