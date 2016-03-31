#!/bin/bash
set -o nounset

lun_id=$1

CLIDK=/root/SM/OceanStor/clidk.jar
ipaddr=146.240.104.61
user='admin'
passwd='Admin@storage'

check_num (){
        arg_name=$1
	tmp=`echo $2 | sed 's/[0-9]//g'`
        [ -n "${tmp}" ] && { echo "Args(${arg_name}) must be interger!";exit 1; }
}

check_num "lun_id" $lun_id


# delete lun
java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "dellun -force -lun ${lun_id}" 


