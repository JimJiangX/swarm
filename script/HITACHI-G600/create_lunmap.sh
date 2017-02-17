#!/bin/bash
set -o nounset

Instance_ID=$1
lun_id=$2
hostname=$3
hostlun_id=$4

port_list=`sudo raidcom get host_grp -I${Instance_ID} -allports | grep -w "${hostname}" | awk '{print $1}'`
if [ "${port_list}" == '' ]; then
	echo "could not find the hstgrp ${hostname}"
	exit 2
fi

for port in ${port_list}
do
	sudo raidcom add lun -I${Instance_ID} -port ${port} ${hostname} -ldev_id ${lun_id} -lun_id ${hostlun_id}
	if [ $? -ne 0 ]; then
		echo "create port(${port}) ldev(${lun_id} mapping to ${hostname} failed!"
		exit 2
	fi
	#while(( $loop<=10 ))
	#do
	#	if [ ${loop} -gt 10  ]; then
	#		echo "create lun mapping faild !"
	#		# if timeout over exit 1
	#		exit 1
	#	fi
	#	auhgmap -unit ${admin_unit} -refer | grep -w ${port} | grep -w ${hostname}

	#	if [ $? -eq 0 ]; then
	#		echo "create lun mapping succeeded!"
	#		break
	#	fi
	#	let "loop++"
	#	sleep 3
	#done
done

