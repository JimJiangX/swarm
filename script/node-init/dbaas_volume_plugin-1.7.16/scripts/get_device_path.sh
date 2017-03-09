#!/bin/bash
set -o nounset

VENDOR=$1
HLUN_ID=$2

check() {
#timeout 3*2=6(sec)
loop=0
while(( $loop<=3 ))
do
	n=`lsscsi -i *:*:*:${HLUN_ID} | awk '{print $NF}' | uniq | wc -l`	
	if [ ${n} -ne 1 ]; then
		echo "Please start multipatchd.service and set \"user_friendly_names no\" in /etc/multipath.conf"
		exit 2
	fi
        let "loop++"
        sleep 2
done
}

#timeout 18*3=54(sec)
loop=0
while(( $loop<=17 ))
do
	check
	mdev_name=`lsscsi -i *:*:*:${HLUN_ID} | awk '{print $NF}' | uniq`
	name_char_count=`echo -n ${mdev_name} | wc -m`
	if [ "${mdev_name}" != '' ] && [ ${name_char_count} -eq 33 ]; then 
		echo /dev/mapper/${mdev_name}
		exit 0
	fi
        let "loop++"
        sleep 3
done

echo "can't find device !"
exit 1

