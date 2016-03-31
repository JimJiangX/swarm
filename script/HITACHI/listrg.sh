#!/bin/bash
set -o nounset

unit=AMS2100_83004824

output=`mktemp /tmp/XXXXX`


for rg_id in "$@"
do
	aurgref -unit ${unit} -m -detail ${rg_id} > ${output}
	total_mb=`cat ${output} | grep "Total Capacity" | awk '{print $4}'`
	free_mb=`cat ${output}  | grep "Free Capacity" | awk '{print $4}'`
	stat=`cat ${output} | grep "Status" | head -n 1 | awk '{print $3}'`
        lun_num=`cat ${output} | grep "Defined LU Count" | awk '{print $5}'`
	echo "${rg_id}" "${total_mb%.*}" "${free_mb%.*}" "${stat}" "${lun_num}"
	rm -f ${output}
done
