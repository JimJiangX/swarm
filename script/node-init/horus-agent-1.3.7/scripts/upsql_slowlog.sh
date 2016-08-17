#!/bin/bash
set -o nounset

if [ $# -ne 3 ];then
	echo "must eqaul to 3"
	exit 2
fi

INSTANCE=$1
USER=$2
PASSWD=$3

LIMIT_COUNT=100
SLOWLOG_FILE=/${INSTANCE}_LOG_LV/slow-query.log
TMPFILE=/tmp/${INSTANCE}_slowlog.data

EXEC_BIN=`which pt-query-digest 2>/dev/null`
if [ "${EXEC_BIN}" == '' ]; then
	echo "not find pt-query-digest"
	exit 4
fi

if [ ! -f ${SLOWLOG_FILE} ]; then
	echo "not find ${SLOWLOG_FILE}"
	exit 4
fi

${EXEC_BIN} --output json  --limit=${LIMIT_COUNT} ${SLOWLOG_FILE} >${TMPFILE}  2>/dev/null
if [ $? -ne 0 ];then
	echo "exec pt-query-digest --output json  --limit=${LIMIT_COUNT} ${SLOWLOG_FILE} failed"
	rm  $TMPFILE
	exit 2
fi

sed /^[[:space:]]*$/d ${TMPFILE} 2>/dev/null

rm $TMPFILE


