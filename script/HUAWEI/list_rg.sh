#!/bin/bash
set -o nounset

ipaddr=$1
user=$2
passwd=$3
shift 3

CLIDK=/root/SM/OceanStor/clidk.jar
lun_output=`mktemp /tmp/XXXXX`
output=`mktemp /tmp/XXXXX`

check_num (){
        arg_name=$1
	tmp=`echo $2 | sed 's/[0-9]//g'`
        [ -n "${tmp}" ] && { echo "Args(${arg_name}) must be interger!";exit 1; }
}

for rg_id in "$@"
do
	check_num "rg" $rg_id
done


# store showlun output
java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "showlun" | sed '1,6d; $d'| sed '/^admin/d; /^===/d' > ${lun_output}

for rg_id in "$@"
do
	java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "showrg -rg $rg_id" > $output
	total_mb=`cat $output | grep "Total" | awk '{print $4}' | tr -d '$' | tr -d '\r'`
	free_mb=`cat $output | grep "Free" | awk '{print $4}' | tr -d '$' | tr -d '\r'`
	stat=`cat $output | grep "Status" | awk '{print $3}' | tr -d '$' | tr -d '\r'`
        lun_num=`cat $lun_output | awk '$2 == '$rg_id'' | wc -l`
	echo "${rg_id}" "${total_mb}" "${free_mb}" "${stat}" "${lun_num}"
	rm -rf $output
done

rm -rf $lun_output


