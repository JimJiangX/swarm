#!/bin/bash
set -o nounset

container_name=$1
count=`ps -ef | grep "/usr/local/mysql/bin/mysqld --defaults-file" | grep ${container_name} | wc -l`
if [ $count -eq 1 ]; then
	status="passing"
else
	status="critical"
fi 

echo $status
