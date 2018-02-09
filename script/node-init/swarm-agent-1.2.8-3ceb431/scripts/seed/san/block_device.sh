#!/bin/bash
set -o nounset

VENDOR=$1
shift
#$@ is HLUN_ID

check() {
	n=`lsscsi -i *:*:*:${HLUN_ID} | awk '{print $NF}' | uniq | wc -l`	
	if [ ${n} -ne 1 ]; then
		#echo "Please set "user_friendly_names no" in /etc/multipath.conf"
		exit 0
	fi
}

get_devname() {
	#timeout 10*6=60(sec)=1(min)
	loop=0
	while(( $loop<=9 ))
	do
		check
		local name=`lsscsi -i *:*:*:${HLUN_ID} | awk '{print $NF}' | uniq`
		if [ "${name}" != '' ]; then 
			dev_name=${name}
			return
		fi
	        let "loop++"
	        sleep 6
	done
	echo "cannot find multipath subdevice"
	exit 3
}

#flush multipath device
for HLUN_ID in $@
do
	get_devname
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
				sleep 3
			fi
		done
	else
		echo "cannot find multipath device"
		exit 2
	fi
done

#remove lsscsi device
for HLUN_ID in $@
do
	for name in `lsscsi | grep ":${HLUN_ID}]" | grep "${VENDOR}" | awk '{print $1}' | sed  's/\[//g; s/\]//g'`
	do
		name=`echo ${name} | sed 's/\:/\ /g'`
		echo "scsi remove-single-device ${name}" > /proc/scsi/scsi
	done
done

