package api

import (
	"fmt"
	"net/http"
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
	invaildParamsError

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
	chinese string
}

func (ec errCode) String() string {
	return fmt.Sprintf("%-10d:	%s", ec.code, ec.comment, ec.chinese)
}

func errCodeV2(method string, md model, cg category, serial int, comment, chinese string) errCode {
	ec := errCodeV1(method, md, cg, serial)
	ec.comment = comment
	ec.chinese = chinese

	return ec
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

	// version|model|method|category|serial
	//    2		 2      1      2       3
	newErrCode := func(version, method, model, category, serial int) errCode {
		mod := func(x, base int) int {
			return x % base
		}

		serial = mod(serial, 1000)
		category = mod(category, 100)
		model = mod(model, 100)

		return errCode{
			code: serial + category*1000 + method*100000 + model*1000000 + version*100000000,
		}
	}

	const v1 = 10

	return newErrCode(v1, mt, int(md), int(cg), serial)
}

var errCodeMap map[int]errCode

func getErrCode(code int) errCode {

	return errCodeMap[code]
}
