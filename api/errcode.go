package api

import (
	"fmt"
)

type model int

const (
	_ model = iota
	_NFS
	_Task
	_DC
	_Image
	_Host
	_Networking
	_Service
	_Unit
	_Storage
	_Backup
)

type category int

const (
	internalError category = iota
	urlParamError
	invalidParamsError

	encodeError
	decodeError

	objectNotExist

	dbQueryError
	dbExecError
	dbTxError

	// dbBadConnection
	// badConnection

	resourcesLack
	othersError
)

type errCode struct {
	code    int
	comment string
	chinese string
}

func (ec errCode) String() string {
	return fmt.Sprintf("%-10d:	%s  %s", ec.code, ec.comment, ec.chinese)
}

func (ec errCode) Code(version, model, category, sec int) int {
	mod := func(x, base int) int {
		return x % base
	}

	sec = mod(sec, 1000)
	category = mod(category, 100)
	model = mod(model, 100)

	return sec + category*1000 + model*100000 + version*10000000
}

func errCodeV1(md model, cg category, sec int, comment, chinese string) errCode {
	// version|model|category|serial
	//    2		 2      2       3

	const v1 = 10

	ec := errCode{
		chinese: chinese,
		comment: comment,
	}

	ec.code = ec.Code(v1, int(md), int(cg), sec)

	return ec
}
