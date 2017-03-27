#!/bin/bash
set -o nounset

ipaddr=$1
user=$2
passwd=$3
lun_id=$4

CLIDK=/root/SM/OceanStor/clidk.jar

check_num (){
        arg_name=$1
	tmp=`echo $2 | sed 's/[0-9]//g'`
        [ -n "${tmp}" ] && { echo "Args(${arg_name}) must be interger!";exit 1; }
}

check_num "lun_id" $lun_id


# delete lun
java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "dellun -force -lun ${lun_id}" 


