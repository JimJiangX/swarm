#!/bin/bash

container_name=$1
docker inspect $container_name > /dev/null 2>&1
if [ $? -ne 0 ]; then
	status=critical
fi

docker exec $container_name /root/check_proxy --default-file /DBAASCNF/upsql-proxy.conf > /dev/null 2>&1

if [  $? -eq 0 ];then
	status=passing
else
	status=critical
fi
echo $status
