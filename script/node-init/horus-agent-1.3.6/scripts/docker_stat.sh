#!/bin/bash

# if [ $# -ne 1 ];then
# 	echo "must eqaul to 1"
#   	exit 2
# fi

if [ $# -lt 1 ];then
	echo "must bigger than 1"
  	exit 2
fi

data=`docker stats --no-stream $1 2>/dev/null| tail -n 1|awk '{print $2":"$8}' | tr -d % `
#$2:CPU  % $8: MEM % 
if [ "$data" = "" ];then
	echo "get data fail"
	exit 2
fi

echo $data

