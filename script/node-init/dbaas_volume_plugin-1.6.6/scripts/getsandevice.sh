#!/bin/bash
set -o nounset


vendor=$1
hostlun_id=$2


subdev_name=`lsscsi | grep ":${hostlun_id}]" | grep "${vendor}" | head -n 1 | awk '{print $6}'`

if [ "$subdev_name" != '' ]; then
	dev_name=`multipath -l ${subdev_name} | grep ${vendor} | awk '{print $1}'`
	echo /dev/mapper/$dev_name
else
	echo "can't find device !"
	exit 1
fi


