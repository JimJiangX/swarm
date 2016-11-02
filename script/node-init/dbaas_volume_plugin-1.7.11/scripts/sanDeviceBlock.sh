#!/bin/bash
set -o nounset

vendor=$1
shift

for hostlun_id in $@
do
	subdev_name=`lsscsi | grep ":${hostlun_id}]" | grep "${vendor}" | head -n 1 | awk '{print $6}'`
	if [ "$subdev_name" != '' ]; then
		dev_name=`multipath -l ${subdev_name} | grep ${vendor} | awk '{print $1}'`
		if [ "${dev_name}" != '' ]; then
			loop=0
			max=10
			while(( $loop<=$max ))
			do
				multipath -f ${dev_name} >/dev/null 2>&1
				if [ $? -eq 0 ]; then
					echo "multipath flush ${dev_name} successful"
					break
				else
					let "loop++"
					if [ $loop -ge $max ]; then
						echo "multipath flush device faild"
						exit 1
					fi	
					sleep 1	
				fi
				
			done
			for name in `lsscsi | grep ":${hostlun_id}]" | grep "${vendor}" | awk '{print $1}' | sed  's/\[//g; s/\]//g'`
			do
				name=`echo ${name} | sed 's/\:/\ /g'`
				echo "scsi remove-single-device ${name}" > /proc/scsi/scsi
			done
		else
			echo "cannot find multipath device"
			exit 1
		fi
	else
		echo "cannot find multipath subdevice"
		exit 1
	fi
done


