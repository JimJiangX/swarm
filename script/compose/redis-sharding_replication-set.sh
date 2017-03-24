#!/bin/bash
set -o nounset

if [ $# -ne 2 ];then
        echo "args number must eqaul to 2"
        exit 2
fi

#replicas_num=1
replicas_num=$1
#nodelist=192.168.2.100:6375,192.168.2.101:6375,192.168.2.102:6375
node_list=$2

MODE=sharding_replication
CUR_DIR=`dirname $0`
TOOLS_DIR=${CUR_DIR}/tools


if [ ! -x ${TOOLS_DIR}/redis-trib.rb ]; then
	echo "${TOOLS_DIR}/redis-trib.rb not exist"
	exit 2
fi

nodes=`echo ${node_list} | sed 's/,/\ /g'`
#${TOOLS_DIR}/redis-trib.rb create --replicas ${replicas_num} ${nodes} > /dev/null 2>&1
${TOOLS_DIR}/redis-trib.rb create --replicas ${replicas_num} ${nodes} 
if [ $? != 0 ]; then
	echo "exec ${TOOLS_DIR}/redis-trib.rb create faild"
	exit 3
fi



