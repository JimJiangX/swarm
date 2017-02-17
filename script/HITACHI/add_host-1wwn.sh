#!/bin/bash
set -o nounset

Instance_ID=$1
hostname=$2
# only support 1 wwn
wwn=$3

port_list=`sudo raidcom get port -I${Instance_ID} 2> /dev/null | sed '1d' | awk '{print $1}' | uniq`

if [ "${port_list}" == ''  ]; then
	echo "get port failed!"
	exit 2
fi

for port in ${port_list}
do
	ret=`sudo raidcom get port -I${Instance_ID} -port ${port} | grep -w ${wwn} | wc -l`
	if [ "${ret}" == "1" ]; then
		sudo raidcom add host_grp -I${Instance_ID} -port ${port} -host_grp_name ${hostname}
		if [ $? -ne 0 ]; then
			echo "add host_grp failed!"
			exit 1
		fi
		sudo raidcom add hba_wwn -I${Instance_ID} -port ${port} ${hostname} -hba_wwn ${wwn}
		if [ $? -ne 0 ]; then
			echo "add hba_wwn failed!"
			exit 1
		fi
	fi
done
