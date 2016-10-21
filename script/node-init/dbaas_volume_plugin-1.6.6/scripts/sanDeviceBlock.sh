#!/bin/bash
set -o nounset

vendor=$1
shift

for hostlun_id in $@
do
	subdev_name=`lsscsi | grep ":${hostlun_id}]" | grep "${vendor}" | head -n 1 | awk '{print $6}'`
	if [ "$subdev_name" != '' ]; then
		dev_name=`multipath -l ${subdev_name} | grep ${vendor} | awk '{print $1}'`
		multipath -f ${dev_name}
	fi

	for name in `lsscsi | grep ":${hostlun_id}]" | grep "${vendor}" | awk '{print $1}' | sed  's/\[//g; s/\]//g'`
	do
		name=`echo ${name} | sed 's/\:/\ /g'`
		echo "scsi remove-single-device ${name}" > /proc/scsi/scsi
	done
done
