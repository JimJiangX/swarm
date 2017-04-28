#!/bin/bash
set -o nounset

container_name=$1
username=$2
password=$3

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

${dir}/check_upsql --default-file /${container_name}_DAT_LV/my.cnf --user $username --password $password > /dev/null 2>&1
if [  $? -eq 0 ];then
	status=passing
else
	status=critical
fi 

echo $status

