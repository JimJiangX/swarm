#!/bin/bash
set -o nounset


vendor=$1
hostlun_id=$2

loop=0
while(( $loop<=10 ))
do
	subdev_name=`lsscsi | grep ":${hostlun_id}]" | grep "${vendor}" | head -n 1 | awk '{print $6}'`
	if [ "$subdev_name" != '' ]; then
		dev_name=`multipath -l ${subdev_name} | grep ${vendor} | awk '{print $1}'`
		if [ "${dev_name}" != '' ]; then 
			echo /dev/mapper/$dev_name
			exit 0
		fi
	fi	
        let "loop++"
        sleep 2
done

echo "can't find device !"
exit 1
