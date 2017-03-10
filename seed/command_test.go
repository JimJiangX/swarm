package seed

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecCommand(t *testing.T) {
	script := fmt.Sprintf("df -h %s", "/home")
	data, err := ExecCommand(script)
	assert.Nil(t, err)
	fmt.Println("data:", data)

	script = "sleep 20"
	data, err = ExecCommand(script)
	assert.Equal(t, err, errors.New("Timeout"))
}

func TestExecShellFile(t *testing.T) {
	fpath := "/tmp/test.sh"
	data, err := ExecShellFile(fpath)
	assert.Nil(t, err)
	fmt.Println("test data:", data)

	fpath = "/tmp/testargs.sh"
	data, err = ExecShellFile(fpath, "test1", "test2")
	assert.Nil(t, err)
	fmt.Println("testargs data:", data)

	fpath = "/tmp/testtimeout.sh"
	data, err = ExecShellFile(fpath, "test1", "test2")
	assert.Equal(t, err, errors.New("Timeout"))
}
