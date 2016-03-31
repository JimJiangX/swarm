#!/bin/bash
set -o nounset

ipaddr=$1
user=$2
passwd=$3
lun_id=$4
hostname=$5
hostlun_id=$6

CLIDK=/root/SM/OceanStor/clidk.jar

check_num (){
        arg_name=$1
	tmp=`echo $2 | sed 's/[0-9]//g'`
        [ -n "${tmp}" ] && { echo "Args(${arg_name}) must be interger!";exit 1; }
}

check_num "lun_id" $lun_id

check_num "hostlun_id" $hostlun_id


# get host id
host_id=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "showhost" | sed '1,6d; $d'| sed '/^admin/d; /^===/d' | awk '$2 == "'${hostname}'"{print $1}'`

# create hostmap
java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "addhostmap -host ${host_id} -hostlun ${hostlun_id} -devlun ${lun_id}"

