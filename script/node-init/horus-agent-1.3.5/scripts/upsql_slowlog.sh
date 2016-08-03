#!/bin/bash

if [ $# -ne 1 ];then
	echo "must eqaul to 1"
  	exit 2
fi

INSTANCE=$1
LIMIT_COUNT=100
SLOWLOG_FILE=/DBAASLOG/slow-query.log
TMPFILE=/tmp/${INSTANCE}_slowlog.data

docker exec $INSTANCE pt-query-digest --output json  --limit=${LIMIT_COUNT} ${SLOWLOG_FILE} >${TMPFILE}  2>/dev/null
if [ $? -ne 0 ];then
	echo "exec pt-query-digest --output json  --limit=${LIMIT_COUNT} ${SLOWLOG_FILE} failed"
	rm  $TMPFILE
	exit 2
fi

sed /^[[:space:]]*$/d ${TMPFILE} 2>/dev/null

rm $TMPFILE


