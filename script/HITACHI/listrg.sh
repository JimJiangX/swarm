#!/bin/bash
set -o nounset

admin_unit=$1
shift 1

output=`mktemp /tmp/XXXXX`


for rg_id in "$@"
do
	aurgref -unit ${admin_unit} -m -detail ${rg_id} > ${output}
	if [ $? -ne 0  ]; then
		continue
	fi
	total_mb=`cat ${output} | grep "Total Capacity" | awk '{print $4}'`
	free_mb=`cat ${output}  | grep "Free Capacity" | awk '{print $4}'`
	stat=`cat ${output} | grep "Status" | head -n 1 | awk '{print $3}'`
        lun_num=`cat ${output} | grep "Defined LU Count" | awk '{print $5}'`
	if [ ${free_mb} == '' ] || [ ${total_mb} == '' ]; then
		exit 2
	fi
	echo "${rg_id}" "${total_mb%.*}" "${free_mb%.*}" "${stat}" "${lun_num}"
	rm -f ${output}
done
