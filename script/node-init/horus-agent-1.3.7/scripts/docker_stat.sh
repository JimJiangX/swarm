#!/bin/bash
set -o nounset

container_name=$1

if [ $# -lt 1 ];then
	echo "must bigger than 1"
  	exit 2
fi

data=`docker stats --no-stream ${container_name} 2>/dev/null| tail -n 1|awk '{print $2":"$8}' | tr -d % `
#$2:CPU  % $8: MEM % 
if [ "${data}" == "" ];then
	echo "get data fail"
	exit 2
fi

echo $data

