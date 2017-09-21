#!/bin/bash
set -o nounset

Instance_ID=$1
lun_id=$2


port_list=`sudo raidcom get ldev -I${Instance_ID} -ldev_id ${lun_id} | grep -w "PORTs" | awk -F: '{for(i=1;i<=NF;i++)a[NR,i]=$i}END{for(j=2;j<=NF;j++)for(k=1;k<=NR;k++)printf k==NR?a[k,j] RS:a[k,j] FS}' | awk '{print $1}'`
if [ "${port_list}" == '' ]; then
	echo "could not find the ldev(${lun_id})"
	exit 0
fi

for port in ${port_list}
do
	sudo raidcom delete lun -I${Instance_ID} -port ${port} -ldev_id ${lun_id}
	if [ $? -ne 0 ]; then
		echo "delete port(${port}) ldev(${lun_id} mapping failed!"
		exit 2
	fi
	
done 

