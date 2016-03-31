#!/bin/bash
set -o nounset

rg=$1
lun_name=$2
lun_size=$3

CLIDK=/root/SM/OceanStor/clidk.jar
ipaddr=146.240.104.61
user='admin'
passwd='Admin@storage'

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

