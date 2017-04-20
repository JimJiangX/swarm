package api

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

type model int

const (
	_Cluster model = iota
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
	urlParamError category = iota

	encodeError
	decodeError

	objectNotExist
	objectRace

	dbBadConnection
	badConnection

	resourcesLack

	internalError
)

type errCode struct {
	code    int
	comment string
}

func (ec errCode) String() string {
	return fmt.Sprintf("%-8d:	%s", ec.code, ec.comment)
}

func errCodeV1(method string, md model, cg category, serial int) errCode {
	mt := 9
	switch method {
	case http.MethodGet:
		mt = 0
	case http.MethodHead:
		mt = 1
	case http.MethodPost:
		mt = 2
	case http.MethodPut:
		mt = 3
	case http.MethodPatch:
		mt = 4
	case http.MethodDelete:
		mt = 5
	case http.MethodConnect:
		mt = 6
	case http.MethodOptions:
		mt = 7
	case http.MethodTrace:
		mt = 8
	}

	// version|method|model|category|serial
	//    2		  1      2      2       2
	newErrCode := func(version, method, model, category, serial int) errCode {
		mod := func(x, base int) int {
			return x % base
		}

		serial = mod(serial, 100)
		category = mod(category, 100)
		model = mod(model, 100)

		return errCode{
			code: serial + category*100 + model*10000 + method*1000000 + version*10000000,
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
