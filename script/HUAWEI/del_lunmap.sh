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

# get map id
map_id=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "showhostmap -lun ${lun_id}" | grep -v showhostmap | grep "${lun_id}" | awk '{print $1}'`

# delete host map
if [ "$map_id" != "" ]; then
	java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c "delhostmap -force -map ${map_id}"
else
	echo "can't find lun hostmap !"
	exit 1
fi

