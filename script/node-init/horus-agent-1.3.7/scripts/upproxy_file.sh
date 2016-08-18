#!/bin/bash
set -o nounset


if [ $# -ne 1 ];then
	echo "must eqaul to 1"
  	exit 3
fi

INSTANCE=$1


logfile="/${INSTANCE}_LOG_LV/upproxy.log"
#upsql.error_file_size
logfilesize=`du -b $logfile 2>/dev/null | awk '{print $1}'`

if [ "$logfilesize" == "" ];then
	logfilesize="err"
fi

echo "${logfilesize}"
