#!/bin/bash
#set -o nounset

#Instance_ID=$1
#hostname=$2
#
#port_list=`sudo raidcom get host_grp -I${Instance_ID} -allports | grep -w "${hostname}" | awk '{print $1}'`
#if [ "${port_list}" == '' ]; then
#	echo "could not find the hstgrp ${hostname}"
#	exit 0
#fi
#
#for port in ${port_list}
#do
#	sudo raidcom delete host_grp -I${Instance_ID} -port ${port} ${hostname}
#	if [ $? -ne 0  ]; then
#        	echo "delete host_grp failed!"
#        	exit 2
#	fi
#done

