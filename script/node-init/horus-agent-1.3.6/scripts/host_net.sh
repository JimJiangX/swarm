#!/bin/bash

TEMFILE=/tmp/hostnettemp.data

function getdata()
{
	subdata=`cat $TEMFILE | grep $1 | tail -n 1 | awk '{print $5":"$6}'`
	# $5:rxkB/s $6:txkB/s
	if [ "$subdata" = "" ];then
		subdata="err:err"
	fi
	echo $subdata
}

if [ $# -lt 1 ];then
	echo "must bigger than 1"
  	exit 2
fi

sar -n DEV 1 2 >$TEMFILE

if [ $? -ne 0 ]; then
   echo "sar command err"
   exit  2
fi

for i in $@; do 

  data=`getdata $i`
  gtata=$gtata:$data
done  

newvar=${gtata:1}
echo $newvar

rm $TEMFILE
