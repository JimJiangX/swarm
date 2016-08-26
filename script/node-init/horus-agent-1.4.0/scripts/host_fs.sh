#!/bin/bash

function getfsdata()
{	
	# unit K
	subdata=`df -k $1 2>/dev/null | tail -n 1 | awk '{print $2":"$5}' | sed 's/%//g'`
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

for i in $@; do 
  data=`getfsdata $i`
  gtata=$gtata:$data
done  

newvar=${gtata:1}
echo $newvar
