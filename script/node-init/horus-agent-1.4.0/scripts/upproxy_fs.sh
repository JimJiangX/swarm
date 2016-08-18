#!/bin/bash
set -o nounset

function getfsdata()
{	
	# unit K
	subdata=`df --output=used,avail $1 2>/dev/null | tail -n 1 | awk '{print $1":"$2}'`
	if [ "$subdata" = "" ];then
		subdata="err:err"
	fi
	# $2:total  $6:Use% 
	echo $subdata
}

if [ $# -lt 1 ];then
	echo "must bigger than 1"
  	exit 2
fi

INSTANCE=$1

datafs=/${INSTANCE}_CNF_LV
logfs=/${INSTANCE}_LOG_LV

data=`getfsdata ${datafs}`
log=`getfsdata $logfs`

echo $data:$log
