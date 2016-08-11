#!/bin/bash

if [ $# -ne 3 ];then
	echo "must eqaul to 3"
	exit 2
fi

INSTANCE=$1
USER=$2
PASSWD=$3

LIMIT_COUNT=100
SLOWLOG_FILE=/DBAASLOG/slow-query.log
TMPFILE=/tmp/${INSTANCE}_slowlog.data

running_status=`docker inspect -f "{{.State.Running}}" ${INSTANCE}`
if [ ${running_status} == "false" ]; then
	exit 4
fi

docker exec $INSTANCE pt-query-digest --output json  --limit=${LIMIT_COUNT} ${SLOWLOG_FILE} >${TMPFILE}  2>/dev/null
if [ $? -ne 0 ];then
	echo "exec pt-query-digest --output json  --limit=${LIMIT_COUNT} ${SLOWLOG_FILE} failed"
	rm  $TMPFILE
	exit 2
fi

sed /^[[:space:]]*$/d ${TMPFILE} 2>/dev/null

rm $TMPFILE


