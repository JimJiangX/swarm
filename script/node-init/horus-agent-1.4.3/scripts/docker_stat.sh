#!/bin/bash
set -o nounset

container_name=$1

if [ $# -lt 1 ];then
	echo "must bigger than 1"
  	exit 2
fi

cpu_num=`docker inspect -f '{{.HostConfig.CpusetCpus}}' ${container_name} | awk -F, '{print NF}'`
data=`docker stats --no-stream ${container_name} 2>/dev/null| tail -n 1|awk '{print $2":"$8}' | tr -d % `
cpu_usage=${data%%:*}
#cpu=`echo "sclae=4;$cpu_usage/$cpu_num" | bc`
cpu=`awk 'BEGIN{printf "%.2f",'$cpu_usage'/'$cpu_num'}'`
mem=${data##*:}

data=`echo $cpu":"$mem`
#$2:CPU  % $8: MEM % 
if [ "${cpu}" == "" ] || [ "${mem}" == "" ]; then
	echo "get data fail"
	exit 2
fi

echo $data
