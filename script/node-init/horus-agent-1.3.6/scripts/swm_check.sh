#!/bin/bash

container_name=$1
output=`mktemp /tmp/XXXXX`

docker inspect $container_name > $output 2>&1
if [ $? -ne 0 ]; then
	status=critical
fi

ip_addr=`cat $output | grep IPADDR | awk -F= '{print $2}' | sed 's/",//g'`
port=`cat $output | grep PORT | awk -F= '{print $2}' | sed 's/",//g'`

rm -f $output
curl -X POST http://${ip_addr}:${port}/ping > /dev/null  2>&1

if [  $? -eq 0 ];then
	status=passing
else
	status=critical
fi

echo $status
