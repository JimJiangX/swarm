#!/bin/bash
set -o nounset

Instance_ID=$1
rg_id=$2
lun_id=$3
lun_size=$4
unit=m

sudo raidcom get ldev -I${Instance_ID} -ldev_id ${lun_id} -check_status NOT DEFINED > /dev/null 2>&1
stat_code=$?
if [ ${stat_code} -eq 227 ]; then
	echo "LDEV(${lun_id}) exceeded Maximum LDEV(4095) on this RAID."
	exit 2
elif [ ${stat_code} -ne 0 ]; then
	echo "lun (${lun_id}) is exist!"
	exit 2
fi

sudo raidcom add ldev -I${Instance_ID} -parity_grp_id ${rg_id} -ldev_id ${lun_id} -capacity ${lun_size}${unit}
if [ $? -ne 0 ]; then
        echo "add ldev (${lun_id}) failed !"
        exit 2
fi

loop=0
while(( $loop<=10 ))
do
	sleep 3

	sudo raidcom get ldev -I${Instance_ID} -ldev_id ${lun_id} -check_status BLK
	if [ $? -eq 0 ]; then
		break
	fi
	
	let "loop++"
done

sudo raidcom initialize ldev -I${Instance_ID} -ldev_id ${lun_id}  -operation qfmt
if [ $? -ne 0 ]; then
        echo "format ldev (${lun_id}) failed !"
        exit 2
fi
