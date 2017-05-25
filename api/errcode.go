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
	internalError category = iota
	urlParamError
	invaildParamsError

	encodeError
	decodeError

	objectNotExist

	dbQueryError
	dbExecError
	dbTxError

	// dbBadConnection
	// badConnection

	resourcesLack
)

type errCode struct {
	code    int
	comment string
	chinese string
}

func (ec errCode) String() string {
	return fmt.Sprintf("%-10d:	%s  %s", ec.code, ec.comment, ec.chinese)
}

func errCodeV1(method string, md model, cg category, serial int, comment, chinese string) errCode {
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
	sum := func(version, method, model, category, serial int) int {
		mod := func(x, base int) int {
			return x % base
		}

		serial = mod(serial, 1000)
		category = mod(category, 100)
		model = mod(model, 100)

		return serial + category*1000 + method*100000 + model*1000000 + version*100000000
	}

	const v1 = 10

	return errCode{
		code:    sum(v1, mt, int(md), int(cg), serial),
		chinese: chinese,
		comment: comment,
	}
}
