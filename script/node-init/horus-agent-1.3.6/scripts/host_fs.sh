#!/bin/bash

function getfsdata()
{	
	subdata=`df -m $1 2>/dev/null | tail -n 1|tr -d %| awk '{print $2":"$5}'`
	if [ "$subdata" = "" ];then
		subdata="err:err"
	fi
	# $2:total  $6:Use% 
	echo $subdata
}

# home=`df -m /home | tail -n 1|tr -d %|  awk '{print $2":"$5}'`
# root=`df -m / | tail -n 1|tr -d %|  awk '{print $2":"$5}'`
# echo $home:$root

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