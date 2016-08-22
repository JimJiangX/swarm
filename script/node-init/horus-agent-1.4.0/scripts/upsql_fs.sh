#!/bin/bash
set -o nounset

function getfsdata()
{	
	# unit K
	subdata=`df -k $1 2>/dev/null | tail -n 1 | awk '{print $2":"$5}' | sed 's/%//'`
	if [ "$subdata" = "" ];then
		subdata="err:err"
	fi
	# $2:total  $5:Use% 
	echo $subdata
}

if [ $# -lt 1 ];then
	echo "must bigger than 1"
  	exit 2
fi

INSTANCE=$1

datafs=/${INSTANCE}_DAT_LV
logfs=/${INSTANCE}_LOG_LV

data=`getfsdata ${datafs}`
log=`getfsdata $logfs`

echo $data:$log
