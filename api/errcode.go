package api

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

type model int

const (
	_ model = iota
	_DC
	_NFS
	_Cluster
	_Host
	_Task
	_Unit
	_Image
	_Service
	_Storage
	_Networking
)

type category int

const (
	_ category = iota
	urlParamError
	bodyParamsError

	encodeError
	decodeError

	objectNotExist
	objectRace

	dbBadConnection
	dbQueryError
	dbExecError
	dbTxError

	badConnection

	resourcesLack

	internalError
)

type errCode struct {
	code    int
	comment string
}

func (ec errCode) String() string {
	return fmt.Sprintf("%-10d:	%s", ec.code, ec.comment)
}

func errCodeV1(method string, md model, cg category, serial int) errCode {
	mt := 0
	switch method {
	case http.MethodGet:
		mt = 1
	case http.MethodHead:
		mt = 2
	case http.MethodPost:
		mt = 3
	case http.MethodPut:
		mt = 4
	case http.MethodPatch:
		mt = 5
	case http.MethodDelete:
		mt = 6
	case http.MethodConnect:
		mt = 7
	case http.MethodOptions:
		mt = 8
	case http.MethodTrace:
		mt = 9
	}

	// version|method|model|category|serial
	//    2		  1      2      2       3
	newErrCode := func(version, method, model, category, serial int) errCode {
		mod := func(x, base int) int {
			return x % base
		}

		serial = mod(serial, 1000)
		category = mod(category, 100)
		model = mod(model, 100)

		return errCode{
			code: serial + category*1000 + model*100000 + method*10000000 + version*100000000,
		}
	}

	const v1 = 10

	return newErrCode(v1, mt, int(md), int(cg), serial)
}

var errcodeMap map[int]errCode

func init() {
	errcodeMap = make(map[int]errCode, 100)
}

func getErrCode(code int) (errCode, error) {
	ec, ok := errcodeMap[code]
	if ok {
		return ec, nil
	}

	return errCode{}, errors.Errorf("err code %d not exist", code)
}

func addErrCode(code int, msg string) {
	errcodeMap[code] = errCode{
		code:    code,
		comment: msg,
	}
}
