#!/bin/bash
set -o nounset

hostname=$1
shift

CLIDK=/root/SM/OceanStor/clidk.jar
ipaddr=146.240.104.61
user='admin'
passwd='Admin@storage'
hg_name='DBaaS_hg'

# get host id
output=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'showhost' | sed '1,6d; $d' | sed '/^admin/d; /^===/d' | awk '$2 == "'${hostname}'"{print $1}' |  tr -d '$\r' | grep -v '^$'`

if [ "$output" == '' ]; then
	echo "can't find host"
	exit 1
else
	host_id=$output
fi


port_id_list=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'showhostport -host '${host_id}'' | sed '1,6d; $d' | sed '/^admin/d; /^===/d' | awk '{print $1}' | tr -d '$\r' | grep -v '^$' `

# delete host port 
for port_id in ${port_id_list} 
do
	java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'delhostport -force -p '${port_id}''
done

# delete host
java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'delhost -force -host '${host_id}' ' 

# check host
output=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'showhost' | sed '1,6d; $d' | sed '/^admin/d; /^===/d' | awk '$2 == "'${hostname}'"{print $1}' |  tr -d '$\r' | grep -v '^$'`
if [ "$output" == '' ]; then
	exit 0
else
	exit 2
fi
