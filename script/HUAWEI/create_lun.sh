#!/bin/bash
set -o nounset

ipaddr=$1
user=$2
passwd=$3
rg=$4
lun_name=$5
lun_size=$6

CLIDK=/root/SM/OceanStor/clidk.jar

check_num (){
        arg_name=$1
	tmp=`echo $2 | sed 's/[0-9]//g'`
        [ -n "${tmp}" ] && { echo "Args(${arg_name}) must be interger!";exit 1; }
}

check_num "rg" $rg

check_num "lun_size" $lun_size

# create lun
java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "createlun -rg ${rg} -n ${lun_name} -lunsize ${lun_size}m  -susize 64" 

lun_id=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "showlun" | sed '1,6d; $d'| sed '/^admin/d; /^===/d' | awk '$7 == "'${lun_name}'"{print $1}'`

echo $lun_id

