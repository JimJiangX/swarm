#!/bin/bash
# version 0.0.1
set -o nounset

CLIDK=/root/SM/OceanStor/clidk.jar


ipaddr=146.240.104.61
user='admin'
passwd='Admin@storage'

hg_name='DBaaS_hg'

hostname=$1
shift


output=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'showhostgroup' | sed '1,6d; $d' | sed '/^admin/d; /^===/d' | awk '$2 == "'${hg_name}'"{print $1}' | tr -d '$' | tr -d '\r'`

if [ "$output" == '' ]; then
	java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'createhostgroup -n '${hg_name}''
	sleep 1
	hg_id=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'showhostgroup' | sed '1,6d; $d' | sed '/^admin/d; /^===/d' | awk '$2 == "'${hg_name}'"{print $1}' | tr -d '$' | tr -d '\r'`
	if [ "$hg_id" == '' ]; then
		exit 1
	fi
else
	hg_id=$output
fi


java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'addhost -group '${hg_id}' -n '${hostname}' -t 0' 

host_id=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'showhost' | sed '1,6d; $d' | sed '/^admin/d; /^===/d' | awk '$2 == "'${hostname}'"{print $1}' | tr -d '$' | tr -d '\r'`
if [ "$host_id" == '' ]; then
	exit 2
fi


n=1
wwn_num=$#
for wwn in "$@" 
do
	java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'addhostport -host '${host_id}' -type 1 -wwn '${wwn}' -n '${hostname}'port0'$n' -mtype 0'
	n=$((n+1))
done


ls_wwn_num=`java -jar ${CLIDK} -devip ${ipaddr} -u ${user} -p ${passwd} -c 'showhostport -host '${host_id}'' | sed '1,6d; $d' | sed '/^admin/d; /^===/d' | tr -d '$\r' | grep -v '^$' | wc -l `

if [ "${wwn_num}" == ${ls_wwn_num} ]; then
	exit 0	
else
	exit 3
fi

