package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestErrCode(t *testing.T) {
	var tests = []struct {
		method string
		code   [3]int
		want   int
	}{
		{code: [...]int{70, 80, 90}, want: 107080090},
		{code: [...]int{7, 8, 9}, want: 100708009},
		{code: [...]int{7, 8, 9}, want: 100708009},
		{code: [...]int{7770, 8880, 9990}, want: 107080990},
	}

	for _, c := range tests {
		ec := errCodeV1(model(c.code[0]), category(c.code[1]), c.code[2], "", "")
		if ec.code != c.want {
			t.Errorf("expect %d but got %d", c.want, ec.code)
		}
	}
}

var (
	modelMap = map[string]model{
		"_DC":  _DC,
		"_NFS": _NFS,
		//	"_Cluster":    _Cluster,
		"_Host":       _Host,
		"_Task":       _Task,
		"_Unit":       _Unit,
		"_Image":      _Image,
		"_Service":    _Service,
		"_Storage":    _Storage,
		"_Networking": _Networking,
	}

	categoryMap = map[string]category{
		"internalError":      internalError,
		"urlParamError":      urlParamError,
		"invalidParamsError": invalidParamsError,
		"encodeError":        encodeError,
		"decodeError":        decodeError,
		"objectNotExist":     objectNotExist,
		"dbQueryError":       dbQueryError,
		"dbExecError":        dbExecError,
		"dbTxError":          dbTxError,
		"resourcesLack":      resourcesLack,
	}
)

func PrintErrCodes() {
	fmt.Println("OK")
}

func ExamplePrintErrCodes() {
	dat, err := readFile("handlers_master.go")
	if err != nil {
		fmt.Println(dat, err)
	}

	r := bytes.NewBuffer(dat)

	key := []byte("errCodeV1(")
	right := byte(')')
	out := make([]_ErrCode, 1, 150)

	out[0] = _ErrCode{
		model:    "_Garden",
		category: "GardenIsNil",
		errCode:  nilGardenCode,
	}

	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		{
			// if contains key
			d := bytes.Index(line, key)
			if d < 0 {
				continue
			}

			line = bytes.TrimSpace(line[d+len(key):])

			last := bytes.LastIndexByte(line, right)
			if last < 0 {
				fmt.Println("warn:", string(line))
				continue
			}

			line = line[:last]
		}

		ec, err := parseErrCodeString(line)
		if err != nil {
			fmt.Println(string(line), err)
		} else {
			out = append(out, ec)
		}
	}

	err = writeFile("./errcodes.txt", out)
	if err != nil {
		fmt.Println(err)
	}

	PrintErrCodes()

	// output:OK
}

func readFile(name string) ([]byte, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	file := filepath.Join(dir, name)

	return ioutil.ReadFile(file)
}

func writeFile(name string, list []_ErrCode) error {
	f, err := os.Create(name)
	if err != nil {
		return err
	}

	defer f.Close()

	for i := range list {
		_, err := f.WriteString(list[i].String())
		if err != nil {
			return err
		}
	}

	return f.Sync()
}

type _ErrCode struct {
	model    string
	category string
	sec      string
	errCode
}

func (ec _ErrCode) String() string {
	return fmt.Sprintf("%d %s %s %s %s\n", ec.code, ec.model, ec.category, ec.comment, ec.chinese)
}

func parseErrCodeString(line []byte) (ec _ErrCode, err error) {
	out := bytes.Split(line, []byte(","))
	if len(out) == 5 {
		ec = _ErrCode{
			model:    string(bytes.TrimSpace(out[0])),
			category: string(bytes.TrimSpace(out[1])),
			sec:      string(bytes.TrimSpace(out[2])),
			errCode: errCode{
				comment: string(out[3]),
				chinese: string(out[4]),
			},
		}
	} else {

		out := bytes.SplitN(line, []byte(","), 4)
		ec = _ErrCode{
			model:    string(bytes.TrimSpace(out[0])),
			category: string(bytes.TrimSpace(out[1])),
			sec:      string(bytes.TrimSpace(out[2])),
			errCode: errCode{
				comment: string(out[3]),
			},
		}

		comment := bytes.TrimSpace(out[3])

		if bytes.Contains(comment, []byte("fmt.Sprintf(")) {
			mid := bytes.Index(comment, []byte("), "))
			if mid > 0 {
				ec.comment = string(comment[:mid+1])
				ec.chinese = string(bytes.TrimSpace(comment[mid+2:]))
			}
		}
	}

	n, err := strconv.Atoi(ec.sec)
	if err != nil {
		return ec, err
	}

	ec.errCode = errCodeV1(modelMap[ec.model],
		categoryMap[ec.category], n,
		ec.comment, ec.chinese)

	return ec, nil
}
