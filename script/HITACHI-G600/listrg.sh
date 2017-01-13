#!/bin/bash
set -o nounset

Instance_ID=$1
shift 1

for rg_id in "$@"
do
	output=`sudo raidcom get parity_grp -I${Instance_ID} | sed '1d' | awk '{ if($2=="'${rg_id}'") print $2":"$5":"$4":"$3 }'`
	if [ $? -ne 0  ] || [ "${output}" == '' ]; then
		continue
	fi
	total_gb=`echo ${output} | awk -F: '{print $2}'`
	total_mb=`echo "scale=2;${total_gb}*1024" | bc`
	free_percent=`echo $output | awk -F: '{print $3}'`
	free_mb=`echo "scale=2;${total_mb}*${free_percent}" | bc`
	stat=NML
        lun_num=`echo ${output} | awk -F: '{print $4}'`
	if [ ! -n "${free_mb}" ] || [ ! -n "${total_mb}" ]; then
		exit 2
	fi
	echo "${rg_id}" "${total_mb%.*}" "${free_mb%.*}" "${stat}" "${lun_num}"
done
