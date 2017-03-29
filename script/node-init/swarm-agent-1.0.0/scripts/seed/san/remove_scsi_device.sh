#!/bin/bash
set -o nounset

vendor=$1
shift

#remove lsscsi device
for hostlun_id in $@
do
	for name in `lsscsi | grep ":${hostlun_id}]" | grep "${vendor}" | awk '{print $1}' | sed  's/\[//g; s/\]//g'`
	do
		name=`echo ${name} | sed 's/\:/\ /g'`
		echo "scsi remove-single-device ${name}" > /proc/scsi/scsi
	done
done
