#!/bin/bash
usage=`free -m | grep '^Mem:' | awk '{print $3/$2*100}'`

if [ "$usage" == "" ];then
		echo "get data fail"
		exit 2
fi
echo "${usage}"
