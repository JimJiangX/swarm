#!/bin/bash
set -o nounset

Instance_ID=$1
lun_id=$2

sudo raidcom get ldev -I${Instance_ID} -ldev_id ${lun_id} -check_status NOT DEFINED
if [ $? -eq 0 ]; then
	echo "The specified ldev (${lun_id} is not defined."
	exit 0
fi

sudo raidcom delete ldev -I${Instance_ID} -ldev_id ${lun_id}
if [ $? -ne 0 ]; then
        echo "delete ldev(${lun_id} failed"
        exit 2
fi

loop=0
while(( $loop<=10 ))
do
	sudo raidcom get ldev -I${Instance_ID} -ldev_id ${lun_id} -check_status NOT DEFINED
	if [ $? -eq 0 ]; then
		echo "delete lun succeeded!"
		exit 0
	fi
	
	let "loop++"
	sleep 3
done

# if timeout over exit 2
echo "delete lun failed !"
exit 2
