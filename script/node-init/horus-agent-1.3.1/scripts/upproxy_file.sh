#!/bin/bash


if [ $# -ne 1 ];then
	echo "must eqaul to 1"
  	exit 3
fi

INSTANCE=$1

logfile="/var/log/upproxy/upproxy.log"
#upsql.error_file_size
logfilesize=`docker exec $INSTANCE du -m $logfile 2>/dev/null | awk '{print $1}'`

if [ "$logfilesize" = "" ];then
	logfilesize="err"
fi

echo $logfilesize
