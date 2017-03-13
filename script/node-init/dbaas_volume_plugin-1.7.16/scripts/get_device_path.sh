#!/bin/bash
set -o nounset

VENDOR=$1
HLUN_ID=$2

#timeout 20*3=60(sec)
loop=0
while(( $loop<=19 ))
do
	n=`lsscsi -i *:*:*:${HLUN_ID} | awk '{print $NF}' | uniq | wc -l`	
	if [ ${n} -ne 1 ]; then
		continue
	fi
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

