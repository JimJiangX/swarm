#!/bin/bash
set -o nounset

container_name=$1
dir=/opt/DBaaS/script
docker inspect $container_name > /dev/null 2>&1
if [ $? -ne 0 ]; then
	status=critical
	echo $status
	exit
fi

running_status=`docker inspect -f "{{.State.Running}}" ${container_name}`
if [ "${running_status}" != "true" ]; then
	status=critical
	echo $status
	exit
fi

${dir}/check_proxy --default-file /${container_name}_CNF_LV/upsql-proxy.conf > /dev/null 2>&1
if [  $? -eq 0 ];then
	status=passing
else
	status=critical
fi
echo $status
