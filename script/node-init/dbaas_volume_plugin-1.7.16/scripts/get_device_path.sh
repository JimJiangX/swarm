#!/bin/bash
set -o nounset

VENDOR=$1
HLUN_ID=$2

check() {
	n=`lsscsi -i *:*:*:${HLUN_ID} | awk '{print $7}' | uniq | wc -l`	
	if [ ${n} -ne 1 ]; then
		echo "Please start multipatchd.service and set \"user_friendly_names no\" in /etc/multipath.conf"
		exit 2
	fi
}

#timeout 10*6=60(sec)=1(min)
loop=0
while(( $loop<=9 ))
do
        sleep 6
	check
	mdev_name=`lsscsi -i *:*:*:${HLUN_ID} | awk '{print $7}' | uniq`
	name_char_count=`echo -n ${mdev_name} | wc -m`
	if [ "${mdev_name}" != '' ] && [ ${name_char_count} -eq 33 ]; then 
		echo /dev/mapper/${mdev_name}
		exit 0
	fi
        let "loop++"
done

echo "can't find device !"
exit 1

